// https://doc.xt.com/#websocket_publicbase
package main

import (
	"context"
	"log"
	"os"
	"time"

	xtws "github.com/liuhengloveyou/xtws-go"
)

func main() {
	os.Setenv("HTTP_PROXY", "http://127.0.0.1:10808")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:10808")

	ctx := context.Background()
	spotWs, err := xtws.NewWsService(ctx, nil, xtws.NewConnConfFromOption(&xtws.ConfOptions{
		URL:          xtws.BaseUrl,
		Key:          "",
		Secret:       "",
		PingInterval: "1s",
	}))
	if err != nil {
		log.Printf("new spot wsService err:%s", err.Error())
		return
	}

	// create callback functions for receive messages
	// spot order book update
	callDeepUpdate := xtws.NewCallBack(func(msg []byte) {
		log.Printf("callDeepUpdate %+v", string(msg))
	})

	callSpotTicker := xtws.NewCallBack(func(msg []byte) {
		log.Printf("callSpotTicker %+v", string(msg))
	})

	// first, set callback
	spotWs.SetCallBack(xtws.ChannelSpotDeep, callDeepUpdate)
	// first, set callback
	spotWs.SetCallBack(xtws.ChannelSpotTicker, callSpotTicker)

	spotWs.SubscribeTicker([]string{"btc_usdt"})
	spotWs.SubscribeDepth([]string{"btc_usdt"}, 5)

	ch := make(chan bool)
	defer close(ch)

	for {
		select {
		case <-ch:
			log.Printf("manual done")
		case <-time.After(time.Second * 1000):
			log.Printf("auto done")
			return
		}
	}
}
