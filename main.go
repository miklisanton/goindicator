package main

import (
	"encoding/json"
	"fmt"
	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
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

type OrderBook struct {
	mu         sync.Mutex
	Asks       *treemap.Map
	Bids       *treemap.Map
	lowestAsk  float64
	highestBid float64
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		Bids: treemap.NewWith(utils.Float64Comparator),
		Asks: treemap.NewWith(utils.Float64Comparator),
	}
}

func (ob *OrderBook) AddBid(price float64, quantity float64) {
	ob.Bids.Put(price, quantity)
	if price > ob.highestBid {
		ob.highestBid = price
	}
}

func (ob *OrderBook) AddAsk(price float64, quantity float64) {
	ob.Asks.Put(price, quantity)
	if price < ob.lowestAsk || ob.lowestAsk == 0 {
		ob.lowestAsk = price
	}
}

func (ob *OrderBook) RemoveBid(price float64) {
	ob.Bids.Remove(price)
	if price == ob.highestBid {
		ob.highestBid, _, _ = ob.GetBestBid()
	}
}

func (ob *OrderBook) RemoveAsk(price float64) {
	ob.Asks.Remove(price)
	if price == ob.lowestAsk {
		ob.lowestAsk, _, _ = ob.GetBestAsk()
	}
}

func (ob *OrderBook) GetBestBid() (float64, float64, bool) {
	if ob.Bids.Empty() {
		return 0, 0, false
	}
	bestBidKey, bestBidValue := ob.Bids.Max()
	return bestBidKey.(float64), bestBidValue.(float64), true
}

func (ob *OrderBook) GetBestAsk() (float64, float64, bool) {
	if ob.Asks.Empty() {
		return 0, 0, false
	}
	bestAskKey, bestAskValue := ob.Asks.Min()
	return bestAskKey.(float64), bestAskValue.(float64), true
}

// Gets depth snapshot from binance, endpoint must be provided
// updates bids and asks and returns lastUpdatedId
func (ob *OrderBook) fetch(endpoint string) (int, error) {
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

	ob.mu.Lock()
	defer ob.mu.Unlock()

	// Copying bids
	ob.Bids.Clear()
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
		ob.AddBid(price, volume)
	}

	// Copying asks
	ob.Asks.Clear()
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
		ob.AddAsk(price, volume)
	}
	return snap.LastUpdateID, nil
}

// Applies depth event to orderbook
func (ob *OrderBook) update(depth BinanceDepthResponse) {
	ob.mu.Lock()
	defer ob.mu.Unlock()
	for _, bid := range depth.Bids {
		price, err := strconv.ParseFloat(bid[0], 64)
		if err != nil {
			log.Printf("Error parsing bid price: %v", err)
			continue
		}
		quantity, err := strconv.ParseFloat(bid[1], 64)
		if err != nil {
			log.Printf("Error parsing bid quantity: %v", err)
			continue
		}
		if quantity == 0 {
			ob.RemoveBid(price)
		} else {
			ob.AddBid(price, quantity)
		}
	}

	for _, ask := range depth.Asks {
		price, err := strconv.ParseFloat(ask[0], 64)
		if err != nil {
			log.Printf("Error parsing ask price: %v", err)
			continue
		}
		quantity, err := strconv.ParseFloat(ask[1], 64)
		if err != nil {
			log.Printf("Error parsing ask quantity: %v", err)
			continue
		}
		if quantity == 0 {
			ob.RemoveAsk(price)
		} else {
			ob.AddAsk(price, quantity)
		}
	}
}

func minKey(m map[float64]float64) (minKey float64, found bool) {
	if len(m) == 0 {
		return 0, false
	}

	minKey = math.Inf(1) // Initialize with positive infinity
	for key := range m {
		if key < minKey {
			minKey = key
		}
	}
	return minKey, true
}

func manageOrderBook(ob *OrderBook, snapEndpoint string, ch <-chan BinanceDepthResponse) {
	lastUpdateID, err := ob.fetch(snapEndpoint)
	afterFetch := true
	if err != nil {
		log.Fatal(err)
	}
	for event := range ch {
		if event.FinalUpdateID < lastUpdateID {
			log.Println("Dropping event ", event.FinalUpdateID)
			continue
		}
		if afterFetch && event.FirstUpdateID <= lastUpdateID && event.FinalUpdateID >= lastUpdateID {
			log.Println("First update after fetch")
			lastUpdateID = event.FinalUpdateID
			ob.update(event)
			afterFetch = false
			continue
		}
		if lastUpdateID != event.PrevUpdateID {
			log.Println("synicing...")
			lastUpdateID, err = ob.fetch(snapEndpoint)
			afterFetch = true
			if err != nil {
				log.Fatal(err)
			}
		} else {
			// Update is in correct order
			ob.update(event)
			lastUpdateID = event.FinalUpdateID
		}
	}
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run main.go <btcusdt>")
	}
	ticker := os.Args[1]
	// Websocket binance api wsEndpoint for retrieving depth and mark price
	wsEndpoint := fmt.Sprintf("wss://fstream.binance.com/ws/%s@depth@500ms", strings.ToLower(ticker))
	// Binance API endpoint to get orderbook snapshot
	snapEndpoint := fmt.Sprintf("https://fapi.binance.com/fapi/v1/depth?symbol=%s&limit=1000", strings.ToUpper(ticker))

	log.Printf("Connecting to %s", wsEndpoint)

	conn, _, err := websocket.DefaultDialer.Dial(wsEndpoint, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	var (
		response BinanceDepthResponse
	)
	// Buffer responses
	bufferedResponses := make(chan BinanceDepthResponse, 256)
	go func() {
		for {
			if err := conn.ReadJSON(&response); err != nil {
				log.Println("ReadJSON:", err)
			}
			bufferedResponses <- response
		}
	}()
	// Fetch orderbook
	ob := NewOrderBook()

	go manageOrderBook(ob, snapEndpoint, bufferedResponses)

	for {
		ob.mu.Lock()
		minAsk, volume, found := ob.GetBestAsk()
		if found {
			log.Println("Best ask: ", minAsk, " quantity: ", volume)
		}
		ob.mu.Unlock()
		time.Sleep(time.Second)
	}
}
