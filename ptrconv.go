package main

func GetRef[T any](v T) *T { return &v }
