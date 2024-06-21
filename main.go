package main

import (
	"goindicator/internal/bot_api"
	rate "goindicator/internal/ratelimiter"
	"goindicator/internal/tracker"
	"log"
	"sync"
	"time"
)

func merge(cs ...chan *bot_api.Notification) chan *bot_api.Notification {
	var wg sync.WaitGroup
	out := make(chan *bot_api.Notification)

	// Start a goroutine for each input channel
	output := func(c chan *bot_api.Notification) {
		for n := range c {
			out <- n
		}
		wg.Done()
	}

	// Add a wait group for each channel
	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	// Start a goroutine to close the output channel once all input channels are closed
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func main() {
	//closeChan := make(chan any)
	//sigChan := make(chan os.Signal, 1)

	//signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	//<-sigChan
	//log.Println("Received interrupt signal, shutting down...")

	// Signal all goroutines to close
	//close(closeChan)
	//time.Sleep(1 * time.Second)

	//for {
	//	log.Println(<-merged)
	//}

	cl := bot_api.NewTLSClient()
	rl := rate.NewLimiter(rate.MaxRequests, rate.Window)

	trackers := make(map[string]*tracker.Tracker)

	out := make(chan *bot_api.Notification)

	go func() {
		for message := range out {
			if message != nil {
				if err := cl.SendWebhookNotification(message); err != nil {
					log.Println(err)
				}
			}
		}
	}()

	timer := time.NewTicker(time.Second * 2)
	for range timer.C {
		new, err := cl.GetSymbols()
		if err != nil {
			log.Println(err)
		}

		// Add new trackers if not exist
		for _, symbol := range new {
			if _, found := trackers[symbol]; !found {
				tr := tracker.NewTracker(symbol, rl)
				trackers[symbol] = tr
				tr.Run(out)
			}
		}

		// Remove trackers that are not in list
		for symbol, tr := range trackers {
			if !contains(new, symbol) {
				tr.Stop()
				delete(trackers, symbol)
			}
		}
	}
}

func contains(strings []string, toFind string) bool {
	for _, str := range strings {
		if str == toFind {
			return true
		}
	}
	return false
}
