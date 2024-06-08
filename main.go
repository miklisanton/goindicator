package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"goindicator/internal/config"
	"goindicator/internal/orderbook"
	"goindicator/internal/retrievers"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var client http.Client

type WebhookPayload struct {
	Message string  `json:"message"`
	OFI     float64 `json:"ofi"`
	Time    string  `json:"time"`
}

func sendWebhookNotification(url string, payload WebhookPayload) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send webhook notification: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	log.Println("Body %s (status %d)", body, resp.StatusCode)

	return nil
}
func init() {
	caBytes, err := os.ReadFile("ca.crt")
	if err != nil {
		log.Fatal(err)
	}

	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(caBytes) {
		log.Fatal("failed to append ca cert")
	}

	cert, err := tls.LoadX509KeyPair("client.crt", "client.key")
	if err != nil {
		log.Fatal(err)
	}

	client = http.Client{
		Timeout: time.Second * 60,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      ca,
				Certificates: []tls.Certificate{cert},
			},
		},
	}
}

func main() {
	resp, err := client.Get("https://go-demo.localtest.me")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body := io.ReadAll(resp.Body)
	log.Println(string(body))

	if len(os.Args) != 2 {
		log.Fatal("Usage: go run main.go <btcusdt>")
	}
	ticker := os.Args[1]
	wsEndpoint := fmt.Sprintf("wss://fstream.binance.com/ws/%s@depth@%dms", strings.ToLower(ticker), config.UpdateTime.Milliseconds())
	snapEndpoint := fmt.Sprintf("https://fapi.binance.com/fapi/v1/depth?symbol=%s&limit=1000", strings.ToUpper(ticker))
	webhookURL := "https://go-demo.localtest.me/webhook"

	retriever := retrievers.NewBinanceRetriever(wsEndpoint)
	go retriever.Stream()

	ob := orderbook.NewOrderBook(config.TickSize)
	go orderbook.ManageOrderBook(ob, snapEndpoint, retriever.Buffer)

	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	timer := time.NewTimer(time.Until(next))

	<-timer.C
	minuteTicker := time.NewTicker(time.Minute)
	log.Println("start")
	bestBidPrice, _, _ := ob.GetBestBid()
	bid := ob.RoundDownToTickSize(bestBidPrice)
	bidVol, _ := ob.GetBidVolume(bid)
	bestAskPrice, _, _ := ob.GetBestAsk()
	ask := ob.RoundUpToTickSize(bestAskPrice)
	askVol, _ := ob.GetAskVolume(ask)
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

		err := sendWebhookNotification(webhookURL, WebhookPayload{Message: "simple message",
			OFI:  bidDelta - askDelta,
			Time: time.Now().Format(time.RFC822)})

		if err != nil {
			log.Println(err)
		}

	}
}
