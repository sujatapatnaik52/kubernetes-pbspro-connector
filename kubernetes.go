package main

import (
	"bytes"
	"encoding/json"
	"errors"	
	"io/ioutil"
	"log"
	"fmt"
	"os"
	"os/exec"         
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type PBSPodList struct {
	Items []PBSPod `json:"items"`
}

type PBSPod struct {
	Metadata PBSPodMetadata `json:"metadata"`
}

type PBSPodMetadata struct {
	Name        string            `json:"name,omitempty"`
	Annotations map[string]string `json:"annotations"`
}


var (
	apiHost           = "127.0.0.1:8001"
	bindingEndpoint  = "/api/v1/namespaces/default/pods/%s/binding/"
	eventEndpoint    = "/api/v1/namespaces/default/events"
	nodeEndpoint     = "/api/v1/nodes"
	podEndpoint      = "/api/v1/pods"
	podNamespace	  = "/api/v1/namespaces/default/pods/"
	watchPodEndpoint = "/api/v1/watch/pods"
)

func postsEvent(event Event) error {
	var bf []byte
	body := bytes.NewBuffer(bf)
	err := json.NewEncoder(body).Encode(event)
	if err != nil {
		return err
	}

	request := &http.Request{
		Body:          ioutil.NopCloser(body),
		ContentLength: int64(body.Len()),
		Header:        make(http.Header),
		Method:        http.MethodPost,
		URL: &url.URL{
			Host:   apiHost,
			Path:   eventEndpoint,
			Scheme: "http",
		},
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return errors.New("Event: Unexpected HTTP status code" + resp.Status)
	}
	return nil
}

func watchUnscheduledPods() (<-chan Pod, <-chan error) {	
	pods := make(chan Pod)
	errc := make(chan error, 1)

	v := url.Values{}
	v.Set("fieldSelector", "spec.nodeName=")	
	v.Add("sort","creationTimestamp asc")
	request := &http.Request{
		Header: make(http.Header),
		Method: http.MethodGet,
		URL: &url.URL{
			Host:     apiHost,
			Path:     watchPodEndpoint,
			RawQuery: v.Encode(),
			Scheme:   "http",
		},
	}	
	request.Header.Set("Accept", "application/json, */*")

	go func() {		
		for {			
			resp, err := http.DefaultClient.Do(request)
			if err != nil {
				errc <- err
				time.Sleep(5 * time.Second)
				continue
			}

			if resp.StatusCode != 200 {
				errc <- errors.New("Error code: " + resp.Status)
				time.Sleep(5 * time.Second)
				continue
			}

			decoder := json.NewDecoder(resp.Body)
			for {
				var event PodWatchEvent
				err = decoder.Decode(&event)
				if err != nil {
					errc <- err
					break
				}

				if event.Type == "ADDED" {
					pods <- event.Object					
				}
			}
		}
	}()
		
	return pods, errc
}

func getUnscheduledPods() (*PodList, error) {
	var podList PodList	

	v := url.Values{}
	v.Set("fieldSelector", "spec.nodeName=")

	request := &http.Request{
		Header: make(http.Header),
		Method: http.MethodGet,
		URL: &url.URL{
			Host:     apiHost,
			Path:     podEndpoint,
			RawQuery: v.Encode(),
			Scheme:   "http",
		},
	}
	request.Header.Set("Accept", "application/json, */*")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	err = json.NewDecoder(resp.Body).Decode(&podList)
	if err != nil {
		return nil, err
	}		
	return &podList, nil
}


func fit(pod *Pod) (string,error) {
	
	var spaceRequired int
	var memoryRequired int
	jobid := ""	
	if pod.Metadata.Annotations["JobID"] == "" {

		//calculate resources

		for _, c := range pod.Spec.Containers {
			milliCores := strings.TrimSuffix(c.Resources.Requests["cpu"], "m")
			cores, err := strconv.Atoi(milliCores)
			if err != nil {
                        	return "Error",err
                        }
			spaceRequired += cores				
		}
		
		ncpus := strconv.Itoa(spaceRequired)

		for _, c := range pod.Spec.Containers {
			if strings.HasSuffix(c.Resources.Requests["memory"], "Mi") {
				milliCores1 := strings.TrimSuffix(c.Resources.Requests["memory"], "Mi")
				cores1, err1 := strconv.Atoi(milliCores1)
				if err1 != nil {
					return "Error",err1
				}
				memoryRequired += cores1
			}
		}	
		mem := strconv.Itoa(memoryRequired)
		mem = mem + "MB"

		argstr := []string{"-l","select=1:ncpus=" + ncpus + ":mem="+mem,"-N",pod.Metadata.Name,"-v","PODNAME="+pod.Metadata.Name,"simple_job.sh"}  
		out, err := exec.Command("qsub", argstr...).Output()
	        if err != nil {
	            log.Fatal(err)
        	    os.Exit(1)
        	}
        	jobid = string(out)
		last := len(jobid) - 1
		jobid = jobid[0:last]
        	time.Sleep(5000 * time.Millisecond)
		
		// Store jobid in pod

		annotation(pod,jobid)	
							    
	} else {				
		jobid = pod.Metadata.Annotations["JobID"] 						
	}
	// find a node
	nodename := findnode(jobid)

	if nodename != "" {
		log.Println("Job Scheduled, associating node " + nodename + " to " + pod.Metadata.Name)
		return nodename, nil
	} 

	out1, err := exec.Command("bash", "-c" ,"qstat -f " + jobid).Output()        
        if err != nil {
            log.Fatal(err)
            os.Exit(1)
        }
	comment := string(out1)
 	splits := strings.Split(comment, "\n")	
	i := 0
	for i >= 0{
            if strings.Contains(splits[i], "comment") {
                break;
            }
            i++;
        }	
	log.Println(pod.Metadata.Name + ":" + splits[i])	 

	timestamp := time.Now().UTC().Format(time.RFC3339)
	event := Event{
		Count:          1,
		Message:        fmt.Sprintf("pod (%s) failed to fit in any node\n", pod.Metadata.Name),
		Metadata:       Metadata{GenerateName: pod.Metadata.Name + "-"},
		Reason:         "FailedScheduling",
		LastTimestamp:  timestamp,
		FirstTimestamp: timestamp,
		Type:           "Warning",
		Source:         EventSource{Component: "PBS-scheduler"},
		InvolvedObject: ObjectReference{
			Kind:      "Pod",
			Name:      pod.Metadata.Name,
			Namespace: "default",
			Uid:       pod.Metadata.Uid,
		},
	}

	postsEvent(event)		
	
	return "",nil
	
	
}


func findnode(jobid string) string {

	returnstring := ""

        out1, err := exec.Command("bash", "-c" ,"qstat -f " + jobid).Output()        
        if err != nil {
            log.Fatal(err)
            os.Exit(1)
        }
	nodevalue := string(out1)
 	splits := strings.Split(nodevalue, " ")	
	flag1 := "job_state"
	flag2 := "substate"
	i := 0
	for i >= 0{
            if splits[i] == flag1 {
                break;
            }
            i++;
        }
	
	j := 0
	for j >= 0{
            if splits[j] == flag2 {
                break;
            }
            j++;
        }
	job_state := splits[i+2]
	last1 := len(job_state) - 1		

	substate := splits[j+2]
	last2 := len(substate) - 1	
	
	if job_state[0:last1] == "R" && substate[0:last2] == "42" {	
	    log.Println("Finding node")
	    word := "exec_host"
            i = 0
            for i >= 0{
                if splits[i] == word {
                    break;
                }
                i++;
            }
	    nodename := splits[i+2]
	    returnstring = strings.SplitAfter(nodename, "/")[0]
	    if returnstring[len(returnstring) - 1:len(returnstring)] == "/" {
	        last := len(returnstring) - 1
	        returnstring = returnstring[0:last]
	    }
	}
	
	return returnstring
}



func annotation(pod *Pod, jobid string) {		
					
	annotations := map[string]string{
		"JobID": jobid,
	}			
	patch := PBSPod{
		PBSPodMetadata{
			Annotations: annotations,
		},
	}
	
	var b []byte
	body := bytes.NewBuffer(b)
	err := json.NewEncoder(body).Encode(patch)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	
	url := "http://" + apiHost + podNamespace + pod.Metadata.Name
	request, err := http.NewRequest("PATCH", url, body)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	
	request.Header.Set("Content-Type", "application/strategic-merge-patch+json")
	request.Header.Set("Accept", "application/json, */*")
	
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}					
	if resp.StatusCode != 200 {
		log.Println(err)
		os.Exit(1)
	}
	
	log.Println("Associating Jobid " + jobid + " to pod " + pod.Metadata.Name)

}


func bind(pod *Pod, node string) error {
	binding := Binding{
		ApiVersion: "v1",
		Kind:       "Binding",
		Metadata:   Metadata{Name: pod.Metadata.Name},
		Target: Target{
			ApiVersion: "v1",
			Kind:       "Node",
			Name:       node,
		},
	}

	var b []byte
	body := bytes.NewBuffer(b)
	err := json.NewEncoder(body).Encode(binding)
	if err != nil {
		return err
	}

	request := &http.Request{
		Body:          ioutil.NopCloser(body),
		ContentLength: int64(body.Len()),
		Header:        make(http.Header),
		Method:        http.MethodPost,
		URL: &url.URL{
			Host:   apiHost,
			Path:   fmt.Sprintf(bindingEndpoint, pod.Metadata.Name),
			Scheme: "http",
		},
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode != 201 {
		return errors.New("Binding: Unexpected HTTP status code" + resp.Status)
	}

	// Shoot a Kubernetes event that the Pod was scheduled successfully.
	message := fmt.Sprintf("Successfully assigned %s to %s", pod.Metadata.Name, node)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	event := Event{
		Count:          1,
		Message:        message,
		Metadata:       Metadata{GenerateName: pod.Metadata.Name + "-"},
		Reason:         "Scheduled",
		LastTimestamp:  timestamp,
		FirstTimestamp: timestamp,
		Type:           "Normal",
		Source:         EventSource{Component: "PBS-scheduler"},
		InvolvedObject: ObjectReference{
			Kind:      "Pod",
			Name:      pod.Metadata.Name,
			Namespace: "default",
			Uid:       pod.Metadata.Uid,
		},
	}
	log.Println(message)
	return postsEvent(event)
}
