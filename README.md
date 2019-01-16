# Integration of PBS Professional with Kubernetes

Integration of PBS Professional with Kubernetes, an industry standard container orchestrator, for PBS to provision and schedule jobs along with docker containers. This integration  will benefit sites in being able to run both HPC workloads as well as container workloads on the same HPC cluster without needing for partitioning into two separate portions. This integration will also allow sites to take advantage of the sophisticated scheduling algorithms in PBS and administer the cluster centrally using a single scheduler with a global set of policies.  
Kubernetes ships with a default scheduler but since the default scheduler does not suit our needs, a custom scheduler which talkes to the server, getting unscheduled pods, then talking to PBS for scheduling them. This custom scheduler can run instead of, or alongside the default scheduler. User can instruct Kubernetes which scheduler to use in the pod definition.

## Usage

Enter the value for `apiHost` in kubernetes.go file for the location of api-server

### How to Build

To Build the custom scheduler, run the follwing command with all source files in working directory:  
go build -a --ldflags '-extldflags "-static"' -tags netgo -installsuffix netgo .

### Run the Scheduler

Run the custom scheduler:

> `./scheduler`

### Create a Pod

> kubectl create -f redis.yaml
> pod "redis" created

The redis pod should be in running state:
> kubectl get pods -o wide

Notice the pod has been deployed to the node where the PBS job with same resource requests is.

> qstat -s
