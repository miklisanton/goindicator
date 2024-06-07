package main

import (
	"fmt"
	"goindicator/internal/config"
	"goindicator/internal/orderbook"
	"goindicator/internal/retrievers"
	"log"
	"os"
	"strings"
	"time"
)

const ()

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run main.go <btcusdt>")
	}
	ticker := os.Args[1]
	wsEndpoint := fmt.Sprintf("wss://fstream.binance.com/ws/%s@depth@%dms", strings.ToLower(ticker), config.UpdateTime.Milliseconds())
	snapEndpoint := fmt.Sprintf("https://fapi.binance.com/fapi/v1/depth?symbol=%s&limit=1000", strings.ToUpper(ticker))

	retriever := retrievers.NewBinanceRetriever(wsEndpoint)
	go retriever.Stream()

	ob := orderbook.NewOrderBook(config.TickSize)
	go orderbook.ManageOrderBook(ob, snapEndpoint, retriever.Buffer)

	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	timer := time.NewTimer(time.Until(next))

	bestBidPrice, _, _ := ob.GetBestBid()
	bid := ob.RoundDownToTickSize(bestBidPrice)
	bidVol, _ := ob.GetBidVolume(bid)
	bestAskPrice, _, _ := ob.GetBestAsk()
	ask := ob.RoundUpToTickSize(bestAskPrice)
	askVol, _ := ob.GetAskVolume(ask)

	<-timer.C
	minuteTicker := time.NewTicker(time.Minute)
	log.Println("start")
	// https://osquant.com/papers/key-insights-limit-order-book/
	for range minuteTicker.C {
		ob.Mu.Lock()
		bidPrev := bid
		bidVolPrev := bidVol
		askPrev := ask
		askVolPrev := askVol
		bestBidPrice, _, _ = ob.GetBestBid()
		bid = ob.RoundDownToTickSize(bestBidPrice)
		bidVol, _ = ob.GetBidVolume(bid)
		bestAskPrice, _, _ = ob.GetBestAsk()
		ask = ob.RoundUpToTickSize(bestAskPrice)
		askVol, _ = ob.GetAskVolume(ask)
		ob.Mu.Unlock()
		bidDelta := ob.GetBidDelta(bid, bidPrev, bidVol, bidVolPrev)
		askDelta := ob.GetAskDelta(ask, askPrev, askVol, askVolPrev)
		log.Println("OFI", bidDelta-askDelta)
		log.Println("Ask", askDelta)
		log.Println("Bid", bidDelta)

	}
}
