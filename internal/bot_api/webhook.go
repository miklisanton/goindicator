package bot_api

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	url2 "net/url"
	"os"
	"time"
)

type Notification struct {
	Message string  `json:"message"`
	OFI     float64 `json:"ofi"`
	Time    string  `json:"time"`
	Ticker  string  `json:"ticker"`
}

type TLSClient struct {
	baseURL string
	client  *http.Client
}

func NewTLSClient() *TLSClient {
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

	return &TLSClient{
		client: &http.Client{
			Timeout: time.Second * 60,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs:      ca,
					Certificates: []tls.Certificate{cert},
				},
			},
		},
		baseURL: "https://go-demo.localtest.me",
	}
}

func (cl *TLSClient) SendWebhookNotification(payload *Notification) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url, err := url2.JoinPath(cl.baseURL, "webhook")
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := cl.client.Do(req)
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

func (cl *TLSClient) GetSymbols() ([]string, error) {
	url, err := url2.JoinPath(cl.baseURL, "symbols")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := cl.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get symbols: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var symbols []string
	if err := json.Unmarshal(body, &symbols); err != nil {
		return nil, err
	}

	return symbols, nil
}
