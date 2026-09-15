package main

import "io"

type OCREngine interface {
	ProcessImage(image io.Reader) (string, error)
}
