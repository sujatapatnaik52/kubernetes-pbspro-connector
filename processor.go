package main

import (	
	"log"
	"sync"
	"time"
)

var processLock = &sync.Mutex{}

func resolveUnscheduledPods(interval int, done chan struct{}, wg *sync.WaitGroup) {			
	for {
		log.Println("\nStarting Scheduler Iteration")
		select {
		case <-time.After(time.Duration(interval) * time.Second):							
			err := reschedulePod()
			if err != nil {
				log.Println(err)
			}
			log.Println("End of Iteration")
		case <-done:
			wg.Done()
			log.Println("Stopped reconciliation loop.")
			return
		}
	}
}

func trackUnscheduledPods(done chan struct{}, wg *sync.WaitGroup) {	
	pods, errc := watchUnscheduledPods()

	for {		
		select {
		case err := <-errc:
			log.Println(err)
		case pod := <-pods:
			processLock.Lock()
			time.Sleep(2 * time.Second)			
			err := schedulePod(&pod)
			if err != nil {
				log.Println(err)
			}
			processLock.Unlock()
		case <-done:
			wg.Done()
			log.Println("Stopped scheduler.")
			return
		}
	}
}

func schedulePod(pod *Pod) error {	
	nodevalue,err := fit(pod)
	if err != nil {
		return err
	}
	if nodevalue == "" {
		return nil
	}
	err = bind(pod, nodevalue)
	if err != nil {
		return err
	}	
	return nil
}

func reschedulePod() error {
	processLock.Lock()
	defer processLock.Unlock()
	pods, err := getUnscheduledPods()
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {		
		err := schedulePod(&pod)
		if err != nil {
			log.Println(err)
		}
	}
	return nil
}
