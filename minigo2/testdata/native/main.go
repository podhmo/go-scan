package main

import "example.com/nat"

// Host-reflect bindings (gen-intrinsics shape): WrapFunc reflect-calls the
// real Go function with by-value marshaling.

func Add() int      { return nat.Add(1, 2) }
func Upper() string { return nat.Upper("go") }
func Concat() string {
	return nat.Concat("a", "b", "c") // variadic
}
func Pair() int {
	a, _ := nat.Pair() // multi-result -> Tuple
	return a
}
func Shift() int      { return nat.Shift(3) }  // int arg converts to uint
func Version() string { return nat.Version }   // ValueOf-bound var/const
func NonFunc() int    { return nat.NonFunc() } // binding a non-func errors at call

func main() {}
