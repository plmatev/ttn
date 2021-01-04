// Copyright © 2017 The Things Network
// Use of this source code is governed by the MIT license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/TheThingsNetwork/ttn/cmd"
	"github.com/TheThingsNetwork/ttn/utils/docs"
)

func main() {
	fmt.Println("# API Reference")
	fmt.Println()
	fmt.Println("The Things Network's backend servers.")
	fmt.Println()
	fmt.Print(docs.Generate(cmd.RootCmd))
}
