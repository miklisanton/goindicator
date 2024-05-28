package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

type BinanceDepthResponse struct {
	FinalUpdateID int        `json:"u"`
	FirstUpdateID int        `json:"U"`
	PrevUpdateID  int        `json:"pu"`
	Asks          [][]string `json:"a"`
	Bids          [][]string `json:"b"`
}

type BinanceDepthSnap struct {
	LastUpdateID int        `json:"lastUpdateId"`
	Asks         [][]string `json:"asks"`
	Bids         [][]string `json:"bids"`
}

type Orderbook struct {
	Asks map[float64]float64
	Bids map[float64]float64
}

// Gets depth snapshot from binance, endpoint must be provided
// updates bids and asks and returns lastUpdatedId
func (o *Orderbook) fetch(endpoint string) (int, error) {
	log.Println("Fetching orderbook ", endpoint)
	resp, err := http.Get(endpoint)
	if err != nil {
		return -1, fmt.Errorf("Error fetching orderbook: %s\n", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1, fmt.Errorf("Readall error: %s\n", err)
	}

	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("Error fetching orderbook: %s\n", resp.Status)
	}

	var snap BinanceDepthSnap
	if err := json.Unmarshal(body, &snap); err != nil {
		return -1, fmt.Errorf("Error unmarshalling json: %s\n", err)
	}
	log.Println(body)

	// Copying bids
	for _, bid := range snap.Bids {
		var (
			price  float64
			volume float64
			err    error
		)
		if price, err = strconv.ParseFloat(bid[0], 64); err != nil {
			log.Println("Error parsing price: ", err)
			continue
		}
		if volume, err = strconv.ParseFloat(bid[1], 64); err != nil {
			log.Println("Error parsing volume: ", err)
			continue
		}
		o.Bids[price] = volume
	}

	// Copying asks
	for _, ask := range snap.Asks {
		var (
			price  float64
			volume float64
			err    error
		)
		if price, err = strconv.ParseFloat(ask[0], 64); err != nil {
			log.Println("Error parsing price: ", err)
			continue
		}
		if volume, err = strconv.ParseFloat(ask[1], 64); err != nil {
			log.Println("Error parsing volume: ", err)
			continue
		}
		o.Asks[price] = volume
	}
	return snap.LastUpdateID, nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run main.go <btcusdt>")
	}
	ticker := os.Args[1]
	// Websocket binance api wsEndpoint for retrieving depth and mark price
	wsEndpoint := fmt.Sprintf("wss://fstream.binance.com/ws/%s@depth", strings.ToLower(ticker))
	// Binance API endpoint to get orderbook snapshot
	snapEndpoint := fmt.Sprintf("https://api.binance.com/api/v3/depth?symbol=%s&limit=1000", strings.ToUpper(ticker))

	ob := Orderbook{
		Bids: make(map[float64]float64),
		Asks: make(map[float64]float64),
	}
	lu, err := ob.fetch(snapEndpoint)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(ob.Bids)
	fmt.Println(lu)

	log.Printf("Connecting to %s", wsEndpoint)

	conn, _, err := websocket.DefaultDialer.Dial(wsEndpoint, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	var (
		response BinanceDepthResponse
	)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer conn.Close()
		for {
			if err := conn.ReadJSON(&response); err != nil {
				log.Fatal("ReadJSON:", err)
			}
			fmt.Println(response.PrevUpdateID)
		}
		wg.Done()
	}()
	wg.Wait()

}
