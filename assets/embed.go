// Package assets exposes the recovered Time Bandits resources.
package assets

import "embed"

// Original contains resources recovered from the supported Amiga disk.
//
//go:embed original/*
var Original embed.FS
