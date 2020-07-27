package main

import (
	"fmt"
	"runtime"
	"time"
)

type A struct {
	val int
	b   *B
}

type B struct {
	val int
	a   *A
	c   *C
}

type C struct {
	val int
}

func f() {
	a := new(A)
	b := new(B)
	c := new(C)
	a.b = b
	b.a = a
	b.c = c
	runtime.SetFinalizer(c, finC)
}

func finC(c *C) {
	fmt.Println("ERROR GC C obj")
}

func fin(a *A) {
	fmt.Println("ERROR GC A obj")
}

func main() {
	go f()
	for i := 0; i < 300; i++ {
		runtime.GC()
		fmt.Println("-----")
		time.Sleep(time.Second)
	}
}
