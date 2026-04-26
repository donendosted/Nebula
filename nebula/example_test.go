package nebula_test

import (
	"fmt"

	"nebula/nebula"
)

func ExampleNewClient() {
	client, err := nebula.NewClient("SB.................................", nebula.NetworkTestnet)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(client.Address() != "")
}
