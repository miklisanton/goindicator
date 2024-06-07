package orderbook

import (
	"encoding/json"
	"fmt"
	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"goindicator/internal/config"
	"goindicator/internal/retrievers"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
)

var (
	fetchingBook = false
)

type BinanceDepthSnap struct {
	LastUpdateID int        `json:"lastUpdateId"`
	Asks         [][]string `json:"asks"`
	Bids         [][]string `json:"bids"`
}

type OrderBook struct {
	Mu       sync.Mutex
	Asks     *treemap.Map
	Bids     *treemap.Map
	tickSize float64
}

func NewOrderBook(tickSize float64) *OrderBook {
	return &OrderBook{
		Bids:     treemap.NewWith(utils.Float64Comparator),
		Asks:     treemap.NewWith(utils.Float64Comparator),
		tickSize: tickSize,
	}
}

func (ob *OrderBook) AddBid(price float64, quantity float64) {
	ob.Bids.Put(price, quantity)
}

func (ob *OrderBook) AddAsk(price float64, quantity float64) {
	ob.Asks.Put(price, quantity)
}

func (ob *OrderBook) RemoveBid(price float64) {
	ob.Bids.Remove(price)
}

func (ob *OrderBook) RemoveAsk(price float64) {
	ob.Asks.Remove(price)
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

// GetBidVolume calculates bid volume starting from price level
func (ob *OrderBook) GetBidVolume(price float64) (float64, bool) {
	highestBid, _, ok := ob.GetBestBid()
	if !ok {
		return 0, false
	}
	if price > highestBid {
		return 0, false
	}
	iterator := ob.Bids.Iterator()
	vol := 0.0
	for iterator.Next() {
		k := iterator.Key().(float64)
		if k >= price {
			vol += iterator.Value().(float64)
		}
	}
	return vol, true
}

// GetAskVolume calculates ask volume up to price level
func (ob *OrderBook) GetAskVolume(price float64) (float64, bool) {
	lowestAsk, _, ok := ob.GetBestAsk()

	if !ok {
		return 0, false
	}
	if price < lowestAsk {
		return 0, false
	}
	iterator := ob.Asks.Iterator()
	vol := 0.0
	for iterator.Next() {
		k := iterator.Key().(float64)
		if k <= price {
			vol += iterator.Value().(float64)
		}
	}
	return vol, true
}

// RoundDownToTickSize rounds a price down to the nearest valid tick size
func (ob *OrderBook) RoundDownToTickSize(price float64) float64 {
	return math.Floor(price/ob.tickSize) * ob.tickSize
}

// RoundUpToTickSize rounds a price up to the nearest valid tick size
func (ob *OrderBook) RoundUpToTickSize(price float64) float64 {
	return math.Ceil(price/ob.tickSize) * ob.tickSize
}
func (ob *OrderBook) fetch(endpoint string) (int, error) {
	ob.Mu.Lock()
	defer ob.Mu.Unlock()
	fetchingBook = true
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
	maxPrice, _ := ob.Asks.Max()
	minPrice, _ := ob.Bids.Min()
	log.Println("Snapshot in range ", minPrice, "->", maxPrice)
	return snap.LastUpdateID, nil
}

func (ob *OrderBook) update(depth *retrievers.BinanceDepthResponse) {
	ob.Mu.Lock()
	defer ob.Mu.Unlock()
	//log.Printf("Applying update: U=%d, u=%d, pu=%d, Latency=%d ms", depth.FirstUpdateID, depth.FinalUpdateID, depth.PrevUpdateID, depth.Latency.Milliseconds())
	if len(depth.Bids) == 0 || len(depth.Asks) == 0 {
		log.Println("Warning: empty bids or asks")
		return
	}
	for _, bid := range depth.Bids {
		if bid[0] == "" || bid[1] == "" {
			log.Println("Warning: empty price or volume")
			continue
		}
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
		if ask[0] == "" || ask[1] == "" {
			log.Println("Warning: empty price or volume")
			continue
		}
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
	bestBidPrice, bestBidQty, _ := ob.GetBestBid()
	bestAskPrice, bestAskQty, _ := ob.GetBestAsk()
	if bestAskPrice < bestBidPrice {
		log.Println("Critical Warning: lowest ask is less than highest bid")
		log.Printf("Best bid: %f quantity: %f | Best ask: %f quantity: %f", bestBidPrice, bestBidQty, bestAskPrice, bestAskQty)
	}
}

func (ob *OrderBook) GetBidDelta(bid, bidPrev, bidVol, bidVolPrev float64) (bidDelta float64) {
	if bid > bidPrev {
		bidDelta = bidVol
	} else if bid < bidPrev {
		bidDelta = -bidVolPrev
	} else {
		bidDelta = bidVol - bidVolPrev
	}
	return bidDelta
}

func (ob *OrderBook) GetAskDelta(ask, askPrev, askVol, askVolPrev float64) (askDelta float64) {
	if ask > askPrev {
		askDelta = -askVol
	} else if ask < askPrev {
		askDelta = askVol
	} else {
		askDelta = askVol - askVolPrev
	}
	return askDelta
}

func cleanBuffer(ob *OrderBook, ch <-chan *retrievers.BinanceDepthResponse) {
	ob.Mu.Lock()
	defer ob.Mu.Unlock()
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func ManageOrderBook(ob *OrderBook, snapEndpoint string, ch <-chan *retrievers.BinanceDepthResponse) {
	cleanBuffer(ob, ch)
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
			log.Println("Previous id missmatch, syncing...")
			cleanBuffer(ob, ch)
			lastUpdateID, err = ob.fetch(snapEndpoint)
			if err != nil {
				log.Fatal(err)
			}
			fetchingBook = false
			afterFetch = true
		} else if !fetchingBook {
			// Update is in correct order
			if event.Latency >= 2*config.UpdateTime {
				// Fetch if update took too long
				log.Println("Too long update response from ws, syncing...")
				lastUpdateID, err = ob.fetch(snapEndpoint)
				if err != nil {
					log.Fatal(err)
				}
				fetchingBook = false
				afterFetch = true
			} else {
				ob.update(event)
				lastUpdateID = event.FinalUpdateID
			}
		}
	}
}
