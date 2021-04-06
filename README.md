# Kubernetes Connector for PBS Professional

Integration of PBS Professional with Kubernetes for PBS Pro to provision and schedule jobs along with docker containers. This integration will benefit sites in being able to run both HPC workloads as well as container workloads on the same HPC cluster without needing for partitioning into two separate portions. This integration will also allow sites to take advantage of the sophisticated scheduling algorithms in PBS Pro and administer the cluster centrally using a single scheduler with a global set of policies. Kubernetes ships with a default scheduler but since the default scheduler does not suit our needs, a custom scheduler which talks to the server, getting unscheduled pods, then talking to PBS Pro for scheduling theem. In theory, user can instruct Kubernetes which scheduler to use in the pod definition. This Integration is also achieved using PBS Pro hooks.  

A hook is a block of Python code that PBS Pro executes at certain events, for example, when a job is queued. Each hook can accept (allow) or reject (prevent) the action that triggers it.

## Installation 

### Requirements
1. [PBS Professional](https://github.com/PBSPro/pbspro) is installed and configured. 
2. [Kubernetes](https://github.com/kubernetes/kubernetes) is installed and configured on PBS Server host. 
3. kubectl command is assumed to be installed in /bin (as defined in kubernetes.PY)
4. Add kubeconfig files to root's home directory

### Steps
Clone the Kubernetes PBS Pro Connector repository to the host. 
```bash
git clone https://github.com/PBSPro/kubernetes-pbspro-connector.git
```

Change directory to kubernetes-pbspro-connector folder. 
```bash
cd kubernetes-pbspro-connector
```

Update `pbs_kubernetes.PY` with values for attributes like schedulername, namespace, kubeconfig_file path, kubectl_path executable path and dynamic_pod_path 

The default values are as follows:

```bash
NON_DEFAULT_NAMESPACE = "default"
schedulerName = ""
serviceAccountName = ""
kubeconfig_file = os.path.join(os.sep, "home", euser, ".kube", "config")
kubectl_path = os.path.join(os.sep, "usr", "bin", "kubectl")
dynamic_pod_path = os.path.join(os.sep, "home", euser)
```

Install PBS Pro hook and config file
```bash
qmgr << EOF
create hook pbs-kubernetes
set hook pbs-kubernetes event = execjob_end
set hook pbs-kubernetes event += execjob_begin
import hook pbs-kubernetes application/x-python default pbs_kubernetes.PY
EOF
```

Change directory to scheduler folder. 
```bash
cd scheduler
```
Build the custom scheduler
```bash
go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo .  
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
Add lebel sched_name=true to this namespace 
```bash
kubectl label ns redis PBS_custom_sched=true
```
Note: The custom scheduler will schedule pods from more than one namespace.
Add lables to namespaces with value name being same as sched_name
Because the scheduler does the following to identify the list of namespaces supported for scheduling
```bash
kubectl get ns -l sched_name
```
Update the scheduler name
```bash
# in file scheduler/kubernetes.go update line:
sched_name = "PBS_custom_sched"
```

Update the service account to be used by the scheduler
```bash
# in file scheduler/main.go update line:
serviceacc := "default"
```
Update the queue name
```bash
# in file scheduler/kubernetes.go update line:
queue_name = "reservationK8"
```
Ensure the path to kubectl is set correctly in scheduler/main.go and path to qstat and qsub is set properly in scheduler/kubernetes.go

The default values set in scheduler/kubernetes.go are as follows:
```bash
kubectl_path     = "/usr/bin/kubectl"
qsub_path        = "/opt/pbs/bin/qsub"
qstat_path       = "/opt/pbs/bin/qstat"
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
