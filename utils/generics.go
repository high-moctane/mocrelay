package utils

func ToRef[T any](v T) *T { return &v }
