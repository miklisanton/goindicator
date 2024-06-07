package retrievers

import (
	"github.com/gorilla/websocket"
	"goindicator/internal/config"
	"log"
	"time"
)

type BinanceDepthResponse struct {
	FinalUpdateID int        `json:"u"`
	FirstUpdateID int        `json:"U"`
	PrevUpdateID  int        `json:"pu"`
	Asks          [][]string `json:"a"`
	Bids          [][]string `json:"b"`
	Latency       time.Duration
}

type BinanceRetriever struct {
	Buffer     chan *BinanceDepthResponse
	EndpointWS string
}

func NewBinanceRetriever(endpoint string) *BinanceRetriever {
	return &BinanceRetriever{
		Buffer:     make(chan *BinanceDepthResponse, 150),
		EndpointWS: endpoint,
	}
}

func (r *BinanceRetriever) Stream() {
	for {
		log.Printf("Connecting to %s", r.EndpointWS)

		conn, _, err := websocket.DefaultDialer.Dial(r.EndpointWS, nil)
		if err != nil {
			log.Fatal("dial:", err)
		}
		defer conn.Close()

		conn.SetPingHandler(func(appData string) error {
			log.Println("Received ping, sending pong")
			return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(config.WriteWait))
		})

		var response BinanceDepthResponse

		for {
			start := time.Now()
			if err := conn.ReadJSON(&response); err != nil {
				log.Println("ReadJSON:", err)
				log.Println("Reconnecting...")
				time.Sleep(time.Second * 2) // Simple backoff strategy
				break
			} else {
				if response.FinalUpdateID == 0 {
					log.Println("Reconnecting due to invalid update ID")
					time.Sleep(time.Second * 2)
					break
				}
				end := time.Now()
				response.Latency = end.Sub(start)
				r.Buffer <- &response
			}
		}
	}
}
