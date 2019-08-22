package taskrunner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	log "github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client/allocrunner/interfaces"
	tinterfaces "github.com/hashicorp/nomad/client/allocrunner/taskrunner/interfaces"
	"github.com/hashicorp/nomad/client/consul"
	"github.com/hashicorp/nomad/client/taskenv"
	agentconsul "github.com/hashicorp/nomad/command/agent/consul"
	"github.com/hashicorp/nomad/nomad/structs"
)

var _ interfaces.TaskPoststartHook = &scriptCheckHook{}
var _ interfaces.TaskPreKillHook = &scriptCheckHook{}
var _ interfaces.TaskExitedHook = &scriptCheckHook{}
var _ interfaces.TaskStopHook = &scriptCheckHook{}

type scriptCheckHookConfig struct {
	alloc  *structs.Allocation
	task   *structs.Task
	consul consul.ConsulServiceAPI

	// Restarter is a subset of the TaskLifecycle interface
	restarter agentconsul.TaskRestarter

	logger log.Logger
}

type scriptCheckHook struct {
	consul       consul.ConsulServiceAPI
	allocID      string
	taskName     string
	restarter    agentconsul.TaskRestarter
	logger       log.Logger
	shutdownCh   chan struct{} // closed when all scripts should shutdown
	shutdownWait time.Duration // max amount of time to wait for all scripts on shutdown

	// The following fields can be changed by Update()
	shutdownDelay time.Duration
	driverExec    tinterfaces.ScriptExecutor
	taskEnv       *taskenv.TaskEnv

	// These maintain state
	scripts        map[string]*scriptCheck
	runningScripts map[string]*taskletHandle

	// Since Update() may be called concurrently with any other hook all
	// hook methods must be fully serialized
	mu sync.Mutex
}

func newScriptCheckHook(c scriptCheckHookConfig) *scriptCheckHook {
	scriptChecks := make(map[string]*scriptCheck)
	for _, service := range c.task.Services {
		for _, check := range service.Checks {
			if check.Type != structs.ServiceCheckScript {
				continue
			}
			serviceID := agentconsul.MakeTaskServiceID(
				c.alloc.ID, c.task.Name, service, false)
			checkID := agentconsul.MakeCheckID(serviceID, check)
			// we're only going to configure the immutable fields of
			// scriptCheck here, with the rest being configured
			// during the Poststart hook so that we have the rest
			// of the task execution environment
			sc := &scriptCheck{
				id:          checkID,
				agent:       c.consul,
				lastCheckOk: true, // start logging on first failure

			}
			// we can't use the promoted fields of tasklet in the struct literal
			sc.allocID = c.alloc.ID
			sc.taskName = c.task.Name
			scriptChecks[checkID] = sc
		}
	}
	if len(scriptChecks) == 0 {
		return nil // the caller should not register this hook
	}
	h := &scriptCheckHook{
		consul:        c.consul,
		allocID:       c.alloc.ID,
		taskName:      c.task.Name,
		scripts:       scriptChecks,
		restarter:     c.restarter,
		shutdownDelay: c.task.ShutdownDelay,
		shutdownWait:  time.Minute, // TODO(tgross): move to const?
		shutdownCh:    make(chan struct{}),
	}
	h.logger = c.logger.Named(h.Name())
	return h
}

func (h *scriptCheckHook) Name() string {
	return "script_checks"
}

func (h *scriptCheckHook) Poststart(ctx context.Context, req *interfaces.TaskPoststartRequest, _ *interfaces.TaskPoststartResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if req.DriverExec == nil {
		return fmt.Errorf("driver doesn't support script checks")
	}

	// Store the TaskEnv for interpolating now and when Updating
	h.driverExec = req.DriverExec
	h.taskEnv = req.TaskEnv
	h.scripts = h.getTaskScriptChecks()

	// Handle starting scripts
	for checkID, script := range h.scripts {
		// If it's already running, cancel and replace
		if oldScript, running := h.runningScripts[checkID]; running {
			oldScript.cancel()
		}
		// Start and store the handle
		h.runningScripts[checkID] = script.run()
	}
	return nil
}

func (h *scriptCheckHook) Update(ctx context.Context, req *interfaces.TaskUpdateRequest, _ *interfaces.TaskUpdateResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get current script checks with request's driver metadata as it
	// can't change due to Updates
	oldScriptChecks := h.getTaskScriptChecks()

	task := req.Alloc.LookupTask(h.taskName)
	if task == nil {
		return fmt.Errorf("task %q not found in updated alloc", h.taskName)
	}

	// Update service hook fields
	h.shutdownDelay = task.ShutdownDelay
	h.taskEnv = req.TaskEnv

	// Create new script checks struct with those new values
	newScriptChecks := h.getTaskScriptChecks()

	// Handle starting scripts
	for checkID, script := range newScriptChecks {
		if _, ok := oldScriptChecks[checkID]; ok {
			// If it's already running, cancel and replace
			if oldScript, running := h.runningScripts[checkID]; running {
				oldScript.cancel()
			}
			// Start and store the handle
			h.runningScripts[checkID] = script.run()
		}
	}
	return nil
}

func (h *scriptCheckHook) PreKilling(ctx context.Context, req *interfaces.TaskPreKillRequest, resp *interfaces.TaskPreKillResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Deregister before killing task
	// TODO(tgross): do we really want to block on this?
	// and what should we do with the error?
	h.deregister()

	// If there's no shutdown delay, exit early
	if h.shutdownDelay == 0 {
		return nil
	}

	h.logger.Debug("waiting before killing task", "shutdown_delay", h.shutdownDelay)
	select {
	case <-ctx.Done():
	case <-time.After(h.shutdownDelay):
	}
	return nil
}

func (h *scriptCheckHook) Exited(context.Context, *interfaces.TaskExitedRequest, *interfaces.TaskExitedResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.deregister()
}

// deregister services from Consul.
func (h *scriptCheckHook) deregister() error {
	// Give script checks time to exit (no need to lock as Run() has exited)
	deadline := time.After(h.shutdownWait)
	for _, script := range h.runningScripts {
		select {
		case <-script.wait():
		case <-deadline:
			return fmt.Errorf("timed out waiting for script checks to run")
		}
	}
	return nil
}

func (h *scriptCheckHook) Stop(ctx context.Context, req *interfaces.TaskStopRequest, resp *interfaces.TaskStopResponse) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.deregister()
}

// interpolateScriptChecks returns an interpolated copy of services and checks with
// values from the task's environment.
func (h *scriptCheckHook) getTaskScriptChecks() map[string]*scriptCheck {
	// Guard against not having a valid taskEnv. This can be the case if the
	// PreKilling or Exited hook is run before Poststart.
	if h.taskEnv == nil || h.driverExec == nil {
		return nil
	}
	newChecks := make(map[string]*scriptCheck)
	for _, orig := range h.scripts {
		sc := orig.Copy()
		sc.exec = h.driverExec
		sc.logger = h.logger
		sc.shutdownCh = h.shutdownCh
		sc.callback = newScriptCheckCallback(sc)
		sc.Command = h.taskEnv.ReplaceEnv(orig.Command)
		sc.Args = h.taskEnv.ParseAndReplace(orig.Args)
	}
	return newChecks
}

// heartbeater is the subset of consul agent functionality needed by script
// checks to heartbeat
type heartbeater interface {
	UpdateTTL(id, output, status string) error
}

// scriptCheck runs script checks via a interfaces.ScriptExecutor and updates the
// appropriate check's TTL when the script succeeds.
type scriptCheck struct {
	id          string
	agent       heartbeater
	lastCheckOk bool // true if the last check was ok; otherwise false
	tasklet
}

func (sc *scriptCheck) Copy() *scriptCheck {
	newSc := sc // TODO(tgross): need to do this
	return newSc
}

// closes over the script check and returns the taskletCallback for
// when the script check executes.
func newScriptCheckCallback(s *scriptCheck) taskletCallback {

	return func(ctx context.Context, params taskletCallbackParams) {

		output := params.output
		code := params.code
		err := params.err

		state := api.HealthCritical
		switch code {
		case 0:
			state = api.HealthPassing
		case 1:
			state = api.HealthWarning
		}

		var outputMsg string
		if err != nil {
			state = api.HealthCritical
			outputMsg = err.Error()
		} else {
			outputMsg = string(output)
		}

		// Actually heartbeat the check
		err = s.agent.UpdateTTL(s.id, outputMsg, state)
		select {
		case <-ctx.Done():
			// check has been removed; don't report errors
			return
		default:
		}

		if err != nil {
			if s.lastCheckOk {
				s.lastCheckOk = false
				s.logger.Warn("updating check failed", "error", err)
			} else {
				s.logger.Debug("updating check still failing", "error", err)
			}

		} else if !s.lastCheckOk {
			// Succeeded for the first time or after failing; log
			s.lastCheckOk = true
			s.logger.Info("updating check succeeded")
		}
	}
}
