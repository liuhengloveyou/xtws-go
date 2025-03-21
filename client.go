package xtws

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/gorilla/websocket"
)

type status int

const (
	disconnected status = iota
	connected
	reconnecting
)

type WsService struct {
	mu        *sync.Mutex
	Logger    *log.Logger
	Ctx       context.Context
	Client    *websocket.Conn
	once      *sync.Once
	loginOnce *sync.Once
	calls     *sync.Map
	conf      *ConnConf
	status    status
	clientMu  *sync.Mutex
}

// ConnConf default URL is spot websocket
type ConnConf struct {
	App              string
	subscribeMsg     *sync.Map
	URL              string
	Key              string
	Secret           string
	MaxRetryConn     int
	SkipTlsVerify    bool
	ShowReconnectMsg bool
	PingInterval     string
}

type ConfOptions struct {
	App              string
	URL              string
	Key              string
	Secret           string
	MaxRetryConn     int
	SkipTlsVerify    bool
	ShowReconnectMsg bool
	PingInterval     string
}

func NewWsService(ctx context.Context, logger *log.Logger, conf *ConnConf) (*WsService, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	defaultConf := getInitConnConf()
	if conf != nil {
		conf = applyOptionConf(defaultConf, conf)
	} else {
		conf = defaultConf
	}

	stop := false
	retry := 0
	var conn *websocket.Conn
	for !stop {
		dialer := websocket.DefaultDialer
		header := http.Header{}
		header.Set("Sec-Websocket-Extensions", "permessage-deflate")

		if conf.SkipTlsVerify {
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}

		c, _, err := dialer.Dial(conf.URL, nil)
		if err != nil {
			if retry >= conf.MaxRetryConn {
				log.Printf("max reconnect time %d reached, give it up", conf.MaxRetryConn)
				return nil, err
			}
			retry++
			log.Printf("failed to connect to %s for the %d time: %s", conf.URL, retry, err)
			time.Sleep(time.Millisecond * (time.Duration(retry) * 500))
			continue
		} else {
			stop = true
			conn = c
		}
	}

	if retry > 0 {
		log.Printf("reconnect succeeded after retrying %d times", retry)
	}

	ws := &WsService{
		mu:        new(sync.Mutex),
		conf:      conf,
		Logger:    logger,
		Ctx:       ctx,
		Client:    conn,
		calls:     new(sync.Map),
		once:      new(sync.Once),
		loginOnce: new(sync.Once),
		status:    connected,
		clientMu:  new(sync.Mutex),
	}

	ws.readMsg()
	go ws.activePing()

	return ws, nil
}

func getInitConnConf() *ConnConf {
	return &ConnConf{
		App:              "spot",
		subscribeMsg:     new(sync.Map),
		MaxRetryConn:     MaxRetryConn,
		Key:              "",
		Secret:           "",
		URL:              BaseUrl,
		SkipTlsVerify:    false,
		ShowReconnectMsg: true,
		PingInterval:     DefaultPingInterval,
	}
}

func applyOptionConf(defaultConf, userConf *ConnConf) *ConnConf {
	if userConf.App == "" {
		userConf.App = defaultConf.App
	}

	if userConf.URL == "" {
		userConf.URL = defaultConf.URL
	}

	if userConf.MaxRetryConn == 0 {
		userConf.MaxRetryConn = defaultConf.MaxRetryConn
	}

	if userConf.PingInterval == "" {
		userConf.PingInterval = defaultConf.PingInterval
	}

	return userConf
}

// NewConnConfFromOption conf from options, recommend using this
func NewConnConfFromOption(op *ConfOptions) *ConnConf {
	if op.URL == "" {
		op.URL = BaseUrl
	}
	if op.MaxRetryConn == 0 {
		op.MaxRetryConn = MaxRetryConn
	}
	return &ConnConf{
		App:              op.App,
		subscribeMsg:     new(sync.Map),
		MaxRetryConn:     op.MaxRetryConn,
		Key:              op.Key,
		Secret:           op.Secret,
		URL:              op.URL,
		SkipTlsVerify:    op.SkipTlsVerify,
		ShowReconnectMsg: op.ShowReconnectMsg,
		PingInterval:     op.PingInterval,
	}
}

func (ws *WsService) GetConnConf() *ConnConf {
	return ws.conf
}

func (ws *WsService) reconnect() error {
	// avoid repeated reconnection
	if ws.status == reconnecting {
		return nil
	}

	ws.clientMu.Lock()
	defer ws.clientMu.Unlock()

	if ws.Client != nil {
		ws.Client.Close()
	}

	ws.status = reconnecting

	stop := false
	retry := 0
	for !stop {
		c, _, err := websocket.DefaultDialer.Dial(ws.conf.URL, nil)
		if err != nil {
			if retry >= ws.conf.MaxRetryConn {
				ws.Logger.Printf("max reconnect time %d reached, give it up", ws.conf.MaxRetryConn)
				return err
			}
			retry++
			log.Printf("failed to connect to server for the %d time, try again later", retry)
			time.Sleep(time.Millisecond * (time.Duration(retry) * 500))
			continue
		} else {
			stop = true
			ws.Client = c
		}
	}

	ws.status = connected

	// resubscribe after reconnect
	ws.conf.subscribeMsg.Range(func(key, value interface{}) bool {
		// key is channel, value is []requestHistory
		if _, ok := value.([]requestHistory); ok {
			for _, req := range value.([]requestHistory) {
				if req.op == nil {
					req.op = &SubscribeOptions{
						IsReConnect: true,
					}
				} else {
					req.op.IsReConnect = true
				}
				if err := ws.baseSubscribe(req.Method, []string{req.Channel}, req.op); err != nil {
					ws.Logger.Printf("after reconnect, subscribe channel[%s] err:%s", key.(string), err.Error())
				} else {
					if ws.conf.ShowReconnectMsg {
						ws.Logger.Printf("reconnect channel[%s] success", key.(string))
					}
				}
			}
		}
		return true
	})

	return nil
}

func (ws *WsService) SetKey(key string) {
	ws.conf.Key = key
}

func (ws *WsService) GetKey() string {
	return ws.conf.Key
}

func (ws *WsService) SetSecret(secret string) {
	ws.conf.Secret = secret
}

func (ws *WsService) GetSecret() string {
	return ws.conf.Secret
}

func (ws *WsService) SetMaxRetryConn(max int) {
	ws.conf.MaxRetryConn = max
}

func (ws *WsService) GetMaxRetryConn() int {
	return ws.conf.MaxRetryConn
}

func (ws *WsService) GetChannelMarkets(channel string) []string {
	var markets []string
	set := mapset.NewSet[string]()
	if _, ok := ws.conf.subscribeMsg.Load(channel); ok {
		for _, v := range set.ToSlice() {
			markets = append(markets, v)
		}
		return markets
	}
	return markets
}

func (ws *WsService) GetChannels() []string {
	var channels []string
	ws.calls.Range(func(key, value interface{}) bool {
		channels = append(channels, key.(string))
		return true
	})
	return channels
}

func (ws *WsService) GetConnection() *websocket.Conn {
	return ws.Client
}

func (ws *WsService) activePing() {
	fmt.Println("ping interval:", ws.conf.PingInterval)

	du, err := time.ParseDuration(ws.conf.PingInterval)
	if err != nil {
		ws.Logger.Printf("failed to parse ping interval: %s, use default ping interval 10s instead", ws.conf.PingInterval)
		du, err = time.ParseDuration(DefaultPingInterval)
		if err != nil {
			du = time.Second * 10
		}
	}

	ticker := time.NewTicker(du)
	defer ticker.Stop()

	for {
		select {
		case <-ws.Ctx.Done():
			return
		case <-ticker.C:
			if ws.status != connected {
				continue
			}

			err = ws.Client.WriteMessage(websocket.TextMessage, []byte("ping"))
			if err != nil {
				ws.Logger.Printf("wsWrite [ping] err:%s", err.Error())
			}
		}
	}
}

var statusString = map[status]string{
	disconnected: "disconnected",
	connected:    "connected",
	reconnecting: "reconnecting",
}

func (ws *WsService) Status() string {
	return statusString[ws.status]
}
