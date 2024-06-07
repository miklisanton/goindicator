package config

import "time"

const (
	pingDelay  = time.Minute * 9
	pongWait   = time.Second * 10
	WriteWait  = time.Second * 5
	maxRetries = 3
	UpdateTime = 100 * time.Millisecond
	TickSize   = 10
)
