package main

import (
	"github.com/beego/beego/v2/core/logs"
	"go-torrent-manager/conf"
	"go-torrent-manager/ipfilter"
	"go-torrent-manager/transfer"
	"go-torrent-manager/withdraw"
	"sync"
)

func main() {
	var wg sync.WaitGroup

	ipfilter.Init(&wg)
	transfer.Init(&wg)
	withdraw.Init(&wg)

	config := conf.Get()
	logs.Info("\U0001F9EC Version:", config.Version)
	wg.Wait()
}
