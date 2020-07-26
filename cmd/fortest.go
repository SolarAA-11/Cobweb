package main

import (
	"fmt"
	"path"
)

type Road struct {
	v int
}

func main() {
	fmt.Println(path.Join("instance", "/root/data"))
}
