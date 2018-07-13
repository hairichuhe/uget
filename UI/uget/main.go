package main

import (
	"fmt"
	"uget"
)

func main() {
	cli := uget.New()
	if err := cli.Run(); err != nil {
		fmt.Println("ERROR:" + err.Error())
	}

	//	url := "http://ahdx.down.chinaz.com/201609/DedeAMPZ_v2.0.1.zip"

	//检查路径是否为可下载文件路径
}
