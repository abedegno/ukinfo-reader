//go:build !linux

package main

func processCPU() cpuUsage { return cpuUsage{} }
