# Kubernetes Connector for PBS Professional

Integration of PBS Professional with Kubernetes for PBS Pro to provision and schedule jobs along with docker containers. This integration will benefit sites in being able to run both HPC workloads as well as container workloads on the same HPC cluster without needing for partitioning into two separate portions. This integration will also allow sites to take advantage of the sophisticated scheduling algorithms in PBS Pro and administer the cluster centrally using a single scheduler with a global set of policies. Kubernetes ships with a default scheduler but since the default scheduler does not suit our needs, a custom scheduler which talks to the server, getting unscheduled pods, then talking to PBS Pro for scheduling them. This custom scheduler has not been verified to run alongside the default Kubernetes scheduler. In theory, user can instruct Kubernetes which scheduler to use in the pod definition. This Integration is also achieved using PBS Pro hooks.  

NOTE: The integration assumes a namespace; Requesting a custom namespace will result in failures or unknown results. 

A hook is a block of Python code that PBS Pro executes at certain events, for example, when a job is queued. Each hook can accept (allow) or reject (prevent) the action that triggers it. A hook can make calls to functions external to PBS Pro.

## Installation 

### Requirements
1. [PBS Professional](https://github.com/PBSPro/pbspro) is installed and configured. 
2. [Kubernetes](https://github.com/kubernetes/kubernetes) is installed and configured on PBS Server host. 
3. kubectl command is assumed to be installed in /bin (as defined in kubernetes.PY)

### Steps
Clone the Kubernetes PBS Pro Connector repository to the host. 
```bash
git clone https://github.com/PBSPro/kubernetes-pbspro-connector.git
```

Change directory to kubernetes-pbspro-connector folder. 
```bash
cd kubernetes-pbspro-connector
```

Update `pbs_kubernetes.CF` with the absolute path to the kubelet config directory. 
The value to --config is the absolute path to the directory that the kubelet will watch for pod manifests to run. This directory will need to be created before starting the scheduler.
```bash
{
    "kubelet_config": "/aboslute/path/to/kubelete_config"
}
```

Install PBS Pro hook and config file
```bash
qmgr << EOF
create hook pbs-kubernetes
set hook pbs-kubernetes event = execjob_end
set hook pbs-kubernetes event += execjob_begin
import hook pbs-kubernetes application/x-python default pbs_kubernetes.PY
import hook pbs-kubernetes application/x-config default pbs_kubernetes.CF
EOF
```

Change directory to scheduler folder. 
```bash
cd scheduler
```

Update `kubernetes.go` by adding the value for `apiHost`, the location of [apiproxy server](https://kubernetes.io/docs/concepts/cluster-administration/proxies/):
Example below will use the apiproxy server port 8001
```bash
# in file scheduler/kubernetes.go update line:
apiHost           = "127.0.0.1:8001"
```

Build the custom scheduler
```bash
go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo .  
```

### Start apiserver proxy
As root, start apiserver proxy. 
Recommend starting apiserver proxy in a different terminal window as it will log information to the screen.

```bash
kubectl proxy
```

### Start scheduler
As root, start the custom scheduler (`kubernetes-pbspro-connector/scheduler/scheduler`).
Recommend starting scheduler in a different terminal window as it will log information to the screen.
```bash
./scheduler
```

You will see periodic messages to the screen logging the start and end of the scheduling iteration. In addition, it will log what job is scheduled. 
```bash
Starting Scheduler Iteration
2020/06/19 12:34:06 End of Iteration
2020/06/19 12:34:06 
Starting Scheduler Iteration
2020/06/19 12:34:26 End of Iteration
2020/06/19 12:34:26 
Starting Scheduler Iteration
2020/06/19 12:34:48 Associating Jobid 11.deepdell2 to pod redis
2020/06/19 12:34:48 Finding node
2020/06/19 12:34:48 Job Scheduled, associating node node001 to redis
2020/06/19 12:34:48 Successfully assigned redis to node001
2020/06/19 12:34:48 End of Iteration
2020/06/19 12:34:48 
Starting Scheduler Iteration
2020/06/19 12:35:08 End of Iteration
```

## Usage
Simple example to assign a cpu and memory request and a cpu and memory limit to a Container. A Container is guaranteed to have as much memory as it requests but is not allowed to use more memory than its limit.

### Using a non default namespace for pods and jobs
Create a namespace
```bash
kubectl create namespace redis
```

Update `kubernetes.go` by adding the value for `nonDefaultNamespace`:
```bash
# in file scheduler/kubernetes.go update line:
nonDefaultNamespace           = "redis"
```

Update `pbs_kubernetes.PY` by adding the value for `NON_DEFAULT_NAMESPACE`:
```bash
NON_DEFAULT_NAMESPACE  = "redis"
```

### Specify cpu and memory requests
To specify a cpu and memory request for a Container, include the resources:requests field in the Container's resource manifest. To specify a cpu and memory limit, include resources:limits. See example below
```bash
cat redis.yaml 

apiVersion: v1
kind: Pod
metadata:
  name: redis
spec:
  containers:
  - name: redis
    image: redis:latest
    resources:
      requests:
        memory: "640Mi"
        cpu: "1"
      limits:
        memory: "700Mi"
        cpu: "3"  
```

### Create and Apply the pod
```bash
kubectl apply -f redis.yaml
pod/redis created
```

### Verify Container pod is running
The Container pod would have been deployed to the node and registered with PBS Professional using the same resource requests. The redis pod should be in running state:
```bash
kubectl get pods -o wide --namespace=redis
NAME    READY   STATUS    RESTARTS   AGE   IP                NODE
redis   1/1     Running   0          31s   XXX.XXX.XXX.XXX   node001
```

The PBS Professional job status will show a job running, whose name matches with the one of the pod.
```bash
# qstat -ans

pbspro: 
                                                            Req'd  Req'd   Elap
Job ID          Username Queue    Jobname    SessID NDS TSK Memory Time  S Time
--------------- -------- -------- ---------- ------ --- --- ------ ----- - -----
11.pbspro    root     workq    redis       14971   1   1  640mb 240:0 R 00:00
   node001/0
   Job run at Fri Jun 19 at 12:28 on (node001:ncpus=1:mem=655360kb)

```

### Terminate Container pod
```bash
kubectl delete pod redis
```
