package main

import "golang.org/x/sys/unix"

func processCPU() cpuUsage {
	var r unix.Rusage
	if unix.Getrusage(unix.RUSAGE_SELF, &r) != nil {
		return cpuUsage{}
	}
	return cpuUsage{true, float64(r.Utime.Sec)*1000 + float64(r.Utime.Usec)/1000, float64(r.Stime.Sec)*1000 + float64(r.Stime.Usec)/1000}
}
