package xtws

import (
	"encoding/json"
	"fmt"
)

type ResponseMsg struct {
	ID     string `json:"id"`
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Method string `json:"method"`
}

type UpdateMsg struct {
	Topic string `json:"topic"` //事件
	Event string `json:"event"` //主题
}

type UpdateTickerMsg struct {
	Topic string `json:"topic"` //事件
	Event string `json:"event"` //主题

	Data struct {
		Symbol   string `json:"s"`  // symbol 交易对
		Time     int64  `json:"t"`  // time 最后成交时间
		PriceChg string `json:"cv"` // priceChangeValue 24⼩时价格变化
		ChgRate  string `json:"cr"` // priceChangeRate 24⼩时价格变化(百分⽐)
		Open     string `json:"o"`  // open 第⼀笔
		Close    string `json:"c"`  // close 最后⼀笔
		High     string `json:"h"`  // high 最⾼价
		Low      string `json:"l"`  // low 最低价
		Quantity string `json:"q"`  // quantity 成交量
		Volume   string `json:"v"`  // volume 成交额
	} `json:"data"`
}

type UpdateDepthMsg struct {
	Topic string `json:"topic"` //事件
	Event string `json:"event"` //主题

	Data struct {
		Symbol   string     `json:"s"` // symbol 交易对
		UpdateID int64      `json:"i"` // updateId
		Time     int64      `json:"t"` // time 时间戳
		Asks     [][]string `json:"a"` // asks 卖盘 [0]价格, [1]数量
		Bids     [][]string `json:"b"` // bids 买盘
	} `json:"data"`
}

func (u *UpdateMsg) GetChannel() string {
	return u.Topic
}

type ServiceError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e ServiceError) Error() string {
	return e.Message
}

func newAuthEmptyErr() error {
	return fmt.Errorf("auth key or secret empty")
}

type WSEvent struct {
	UpdateMsg
}

type ChannelEvent struct {
	Event  string
	Market []string
}

type WebsocketRequest struct {
	Market []string
}

type Request struct {
	Id     string   `json:"id,omitempty"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type Auth struct {
	Method string `json:"method"`
	Key    string `json:"KEY"`
	Secret string `json:"SIGN"`
}

type requestHistory struct {
	Channel string `json:"channel"`
	Method  string `json:"method"`
	op      *SubscribeOptions
}

type APIReq struct {
	ApiKey    string          `json:"api_key"`
	Signature string          `json:"signature"`
	Timestamp string          `json:"timestamp"`
	ReqId     string          `json:"req_id"`
	ReqHeader json.RawMessage `json:"req_header"`
	ReqParam  json.RawMessage `json:"req_param"`
}

type APIResp struct {
	ClientID   string `json:"client_id"`
	ReqID      string `json:"req_id"`
	RespTimeMs int64  `json:"resp_time_ms"`
	Status     int    `json:"status"`
	ReqHeader  struct {
		XGateChannelID string `json:"x-gate-channel-id"`
	} `json:"req_header"`
	Data struct {
		Error  any `json:"error"`
		Result any `json:"result"`
	} `json:"data"`
}
