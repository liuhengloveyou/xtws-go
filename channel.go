package xtws

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type SubscribeOptions struct {
	ID          string `json:"id"`
	IsReConnect bool   `json:"-"`
}

// https://doc.xt.com/#websocket_public_cnlimitDepth
func (ws *WsService) SubscribeDepth(symbols []string, level int) error {
	channels := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		channels = append(channels, fmt.Sprintf("%s@%s,%d", ChannelSpotDeep, symbol, level))
	}

	return ws.newBaseChannel(channels, nil)
}

// https://doc.xt.com/#websocket_public_cntickerRealTime
func (ws *WsService) SubscribeTicker(symbols []string) (channel string, err error) {
	channels := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		channels = append(channels, fmt.Sprintf("%s@%s", ChannelSpotTicker, symbol))
	}

	err = ws.newBaseChannel(channels, nil)

	return
}

func (ws *WsService) Subscribe(channels []string) error {
	for _, channel := range channels {
		if (ws.conf.Key == "" || ws.conf.Secret == "") && authChannel[channel] {
			return newAuthEmptyErr()
		}
	}

	return ws.newBaseChannel(channels, nil)
}

func (ws *WsService) SubscribeWithOption(channels []string, op *SubscribeOptions) error {
	for _, channel := range channels {
		if (ws.conf.Key == "" || ws.conf.Secret == "") && authChannel[channel] {
			return newAuthEmptyErr()
		}
	}

	// msgCh, ok := ws.msgChs.Load(channel)
	// if !ok {
	// 	msgCh = make(chan *UpdateMsg, 1)
	// 	go ws.receiveCallMsg(channel, msgCh.(chan *UpdateMsg))
	// }

	return ws.newBaseChannel(channels, op)
}

func (ws *WsService) UnSubscribe(channels []string) error {
	return ws.baseSubscribe(UnSubscribe, channels, nil)
}

func (ws *WsService) newBaseChannel(channels []string, op *SubscribeOptions) error {
	err := ws.baseSubscribe(Subscribe, channels, op)
	if err != nil {
		return err
	}

	return nil
}

func (ws *WsService) baseSubscribe(method string, channels []string, op *SubscribeOptions) error {

	// hash := hmac.New(sha512.New, []byte(ws.conf.Secret))
	// hash.Write([]byte(fmt.Sprintf("channel=%s&event=%s&time=%d", channel, Subscribe, ts)))
	req := Request{
		Method: method,
		Params: channels,
	}
	// options
	if op != nil {
		req.Id = op.ID
	}

	byteReq, err := json.Marshal(req)
	if err != nil {
		ws.Logger.Printf("req Marshal err:%s", err.Error())
		return err
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()

	err = ws.Client.WriteMessage(websocket.TextMessage, byteReq)
	fmt.Println("baseSubscribe", string(byteReq))
	if err != nil {
		ws.Logger.Printf("wsWrite [%s] err:%s", channels, err.Error())
		return err
	}

	for _, channel := range channels {
		if v, ok := ws.conf.subscribeMsg.Load(channel); ok {
			if op != nil && op.IsReConnect {
				return nil
			}
			reqs := v.([]requestHistory)
			reqs = append(reqs, requestHistory{
				Channel: channel,
				Method:  method,
				op:      op,
			})
			ws.conf.subscribeMsg.Store(channel, reqs)
		} else {
			// avoid saving invalid subscribe msg
			if strings.HasSuffix(channel, ".ping") || strings.HasSuffix(channel, ".time") {
				return nil
			}

			ws.conf.subscribeMsg.Store(channel, []requestHistory{{
				Channel: channel,
				Method:  method,
				op:      op,
			}})
		}
	}
	return nil
}

// readMsg only run once to read message
func (ws *WsService) readMsg() {
	ws.once.Do(func() {
		go func() {
			defer ws.Client.Close()

			for {
				select {
				case <-ws.Ctx.Done():
					ws.Logger.Printf("closing reader")
					return

				default:
					_, rawMsg, err := ws.Client.ReadMessage()
					fmt.Println("readMsg", string(rawMsg))
					if err != nil {
						ws.Logger.Printf("websocket err: %s", err.Error())
						if e := ws.reconnect(); e != nil {
							ws.Logger.Printf("reconnect err:%s", err.Error())
							return
						}
						ws.Logger.Println("reconnect success, continue read message")
						continue
					}
					if bytes.Equal(rawMsg, []byte("pong")) {
						continue
					}

					var msg UpdateMsg
					if err := json.Unmarshal(rawMsg, &msg); err != nil {
						continue
					}

					channel := msg.GetChannel()
					if channel == "" {
						ws.Logger.Printf("channel is empty in message %v", msg)
						continue
					}

					if call, ok := ws.calls.Load(channel); ok {
						call.(CallBack)(rawMsg)
					}
				}
			}
		}()
	})
}

type CallBack func([]byte)

func NewCallBack(f func([]byte)) func([]byte) {
	return f
}

func (ws *WsService) SetCallBack(channel string, call CallBack) {
	if call == nil {
		return
	}
	ws.calls.Store(channel, call)
}

func (ws *WsService) APIRequest(channel string, keyVals map[string]any) error {
	var err error
	ws.loginOnce.Do(func() {
		err = ws.login()
	})

	if err != nil {
		return err
	}

	if (ws.conf.Key == "" || ws.conf.Secret == "") && authChannel[channel] {
		return newAuthEmptyErr()
	}

	ws.readMsg()

	return ws.apiRequest(channel, keyVals)
}

func (ws *WsService) login() error {
	if ws.conf.Key == "" || ws.conf.Secret == "" {
		return newAuthEmptyErr()
	}
	channel := ChannelSpotLogin
	if ws.conf.App == "futures" {
		channel = ChannelFutureLogin
	}

	ws.readMsg()

	return ws.apiRequest(channel, nil)
}

func (ws *WsService) apiRequest(channel string, keyVals map[string]any) error {
	req := Request{
		Method: channel,
		Params: []string{channel},
	}

	byteReq, err := json.Marshal(req)
	if err != nil {
		ws.Logger.Printf("req Marshal err:%s", err.Error())
		return err
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()

	return ws.Client.WriteMessage(websocket.TextMessage, byteReq)
}

func (ws *WsService) generateAPIRequest(channel string, placeParam any, keyVals map[string]any) any {
	reqID := "req_id"
	gateChannelID := "T_channel_id"

	if v, ok := keyVals["req_id"]; ok {
		reqID, _ = v.(string)
	}

	if v, ok := keyVals["X-Gate-Channel-Id"]; ok {
		gateChannelID, _ = v.(string)
	}

	now := time.Now().Unix()

	reqParam, _ := json.Marshal(placeParam)

	message := fmt.Sprintf("api\n%s\n%s\n%d", channel, reqParam, now)

	return APIReq{
		ApiKey:    ws.conf.Key,
		Signature: calculateSignature(ws.conf.Secret, message),
		Timestamp: strconv.Itoa(int(now)),
		ReqId:     reqID,
		ReqHeader: json.RawMessage(fmt.Sprintf(`{"X-Gate-Channel-Id":"%s"}`, gateChannelID)),
		ReqParam:  reqParam,
	}
}

func calculateSignature(secret string, message string) string {
	h := hmac.New(sha512.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
