import Component from '@ember/component';

export default Component.extend({
  jsonSmall: {
    foo: 'bar',
    number: 123456789,
    products: ['Consul', 'Nomad', 'Packer', 'Terraform', 'Vagrant', 'Vault'],
    currentTime: new Date().toISOString(),
    nested: {
      obj: 'ject',
    },
    nonexistent: null,
    huh: undefined,
    isTrue: false,
  },

  jsonLarge: {
    Stop: false,
    Region: 'global',
    Namespace: 'default',
    ID: 'syslog',
    ParentID: '',
    Name: 'syslog',
    Type: 'system',
    Priority: 50,
    AllAtOnce: false,
    Datacenters: ['dc1', 'dc2'],
    Constraints: null,
    TaskGroups: [
      {
        Name: 'syslog',
        Count: 1,
        Update: {
          Stagger: 10000000000,
          MaxParallel: 1,
          HealthCheck: 'checks',
          MinHealthyTime: 10000000000,
          HealthyDeadline: 300000000000,
          ProgressDeadline: 600000000000,
          AutoRevert: false,
          Canary: 0,
        },
        Migrate: null,
        Constraints: [
          {
            LTarget: '',
            RTarget: '',
            Operand: 'distinct_hosts',
          },
        ],
        RestartPolicy: {
          Attempts: 10,
          Interval: 300000000000,
          Delay: 25000000000,
          Mode: 'delay',
        },
        Tasks: [
          {
            Name: 'syslog',
            Driver: 'docker',
            User: '',
            Config: {
              port_map: [
                {
                  tcp: 601.0,
                  udp: 514.0,
                },
              ],
              image: 'balabit/syslog-ng:latest',
            },
            Env: null,
            Services: null,
            Vault: null,
            Templates: null,
            Constraints: null,
            Resources: {
              CPU: 500,
              MemoryMB: 256,
              DiskMB: 0,
              IOPS: 0,
              Networks: [
                {
                  Device: '',
                  CIDR: '',
                  IP: '',
                  MBits: 10,
                  ReservedPorts: [
                    {
                      Label: 'udp',
                      Value: 514,
                    },
                    {
                      Label: 'tcp',
                      Value: 601,
                    },
                  ],
                  DynamicPorts: null,
                },
              ],
            },
            DispatchPayload: null,
            Meta: null,
            KillTimeout: 5000000000,
            LogConfig: {
              MaxFiles: 10,
              MaxFileSizeMB: 10,
            },
            Artifacts: null,
            Leader: false,
            ShutdownDelay: 0,
            KillSignal: '',
          },
        ],
        EphemeralDisk: {
          Sticky: false,
          SizeMB: 300,
          Migrate: false,
        },
        Meta: null,
        ReschedulePolicy: null,
      },
    ],
    Update: {
      Stagger: 10000000000,
      MaxParallel: 1,
      HealthCheck: '',
      MinHealthyTime: 0,
      HealthyDeadline: 0,
      ProgressDeadline: 0,
      AutoRevert: false,
      Canary: 0,
    },
    Periodic: null,
    ParameterizedJob: null,
    Dispatched: false,
    Payload: null,
    Meta: null,
    VaultToken: '',
    Status: 'running',
    StatusDescription: '',
    Stable: false,
    Version: 0,
    SubmitTime: 1530052201331477665,
    CreateIndex: 27,
    ModifyIndex: 27,
    JobModifyIndex: 27,
  },
});
