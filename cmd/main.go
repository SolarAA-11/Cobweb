package main

import (
	"fmt"
	"sync"
)

func main() {
	int2once := make(map[int]*sync.Once)

	int2once[1] = &sync.Once{}
	int2once[2] = &sync.Once{}
	int2once[3] = &sync.Once{}
	int2once[4] = &sync.Once{}

	for i := 1; i <= 4; i++ {
		for k := 0; k < 5; k++ {
			int2once[i].Do(func() {
				fmt.Println(i)
			})
		}
	}

}
