package tracker

import (
	"fmt"
	"goindicator/internal/bot_api"
	"goindicator/internal/config"
	"goindicator/internal/orderbook"
	rate "goindicator/internal/ratelimiter"
	"goindicator/internal/retrievers"
	"log"
	"strings"
	"time"
)

const defaultTickSize = 10.0

type Tracker struct {
	symbol       string
	rl           *rate.Limiter
	ob           *orderbook.OrderBook
	ret          *retrievers.BinanceRetriever
	snapEndpoint string
	closeChan    chan any
}

func NewTracker(symbol string, rl *rate.Limiter) *Tracker {
	wsEndpoint := fmt.Sprintf("wss://fstream.binance.com/ws/%s@depth@%dms", strings.ToLower(symbol), config.UpdateTime.Milliseconds())
	snapEndpoint := fmt.Sprintf("https://fapi.binance.com/fapi/v1/depth?symbol=%s&limit=1000", strings.ToUpper(symbol))

	return &Tracker{
		symbol:       symbol,
		rl:           rl,
		ob:           orderbook.NewOrderBook(symbol, defaultTickSize),
		ret:          retrievers.NewBinanceRetriever(wsEndpoint),
		snapEndpoint: snapEndpoint,
		closeChan:    make(chan any),
	}
}

func (t *Tracker) Run(out chan *bot_api.Notification) {
	log.Println("+", t.symbol)

	go t.ret.Stream(t.closeChan)
	go orderbook.ManageOrderBook(t.ob, t.snapEndpoint, t.ret.Buffer, t.rl, t.closeChan)

	now := time.Now()
	next := now.Truncate(time.Second).Add(time.Second)
	timer := time.NewTimer(time.Until(next))

	go func() {
		select {
		case <-timer.C:
			log.Println("Starting tracker...")
		case <-t.closeChan:
			log.Println("shutting down tracker for", t.symbol)
			return
		}
		minuteTicker := time.NewTicker(time.Second)

		bestBidPrice, _, _ := t.ob.GetBestBid()
		bid := t.ob.RoundDownToTickSize(bestBidPrice)
		bidVol, _ := t.ob.GetBidVolume(bid)
		bestAskPrice, _, _ := t.ob.GetBestAsk()
		ask := t.ob.RoundUpToTickSize(bestAskPrice)
		askVol, _ := t.ob.GetAskVolume(ask)
		// https://osquant.com/papers/key-insights-limit-order-book/
		for {
			select {
			case <-t.closeChan:
				log.Println("shutting down tracker for", t.symbol)
				return
			case <-minuteTicker.C:
				t.ob.Mu.Lock()

				bidPrev := bid
				bidVolPrev := bidVol
				bestBidPrice, _, _ = t.ob.GetBestBid()
				bid = t.ob.RoundDownToTickSize(bestBidPrice)
				bidVol, _ = t.ob.GetBidVolume(bid)
				bidDelta := t.ob.GetBidDelta(bid, bidPrev, bidVol, bidVolPrev)

				askPrev := ask
				askVolPrev := askVol
				bestAskPrice, _, _ = t.ob.GetBestAsk()
				ask = t.ob.RoundUpToTickSize(bestAskPrice)
				askVol, _ = t.ob.GetAskVolume(ask)
				askDelta := t.ob.GetAskDelta(ask, askPrev, askVol, askVolPrev)

				t.ob.Mu.Unlock()

				payload := bot_api.Notification{
					Message: "simple message",
					OFI:     bidDelta - askDelta,
					Ticker:  strings.ToUpper(t.symbol),
					Time:    time.Now().Format(time.RFC822),
				}

				out <- &payload
				log.Printf("DEBUG: %v\n", payload)
			}
		}
	}()
}

func (t *Tracker) Stop() {
	t.closeChan <- struct{}{}
	log.Println("-", t.symbol)
}
