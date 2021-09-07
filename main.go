package main

import (
	beego "github.com/beego/beego/v2/server/web"
	_ "go-torrent-manager/routers"
)

func main() {
	beego.Run()
}
