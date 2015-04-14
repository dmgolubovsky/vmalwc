// The command utility for VM job control

package main

import (
	"os/user"
)

type Job struct {
	kernel string		// which kernel to load
	kvm string		// path to KVM
	libmap []*Library	// toplevel libraries
	steps StepMap		// job steps
	mmegs int		// default memory for VM
	xdisplay int		// if positive, enable guestfwd to X server with given display
	video bool		// if true, video access (X display) is allowed
	audio bool		// if true, audio access (pulseaudio forwarding) is allowed
	wdir string		// working directory of the job
	user *user.User		// host user information
	uimp bool		// if true, import user home directory into the VM
}

type LibMapN map [int] *Library

type Library struct {
	name string		// name of the library that can be used for step reference
	stname string		// name of the step where library was defined
	path string		// host path to the library (directory or volume)
	libtype int		// type of the library (raw, qcow, 9p, reference)
	write bool		// writable
	id string		// id for KVM device definition
	tag string		// tag for 9p, serial for volumes
	save string		// if nonempty, save the volume at this path (ignored for 9p)
	from string		// if nonempty, copy the volume from this path into a new volume
	snap bool		// if true, use this volume in snapshot mode
	refstep string		// for references, address a step whose library is reused, or top if blank
	reflib *Library		// reference to the library in other step
	newsize int		// if positive, the library has to be created anew even if existed - volumes only
}

type StepMap map [string] *Step

type Step struct {
	name string		// step name
	exec string		// will be passed to the kernel command line encoded
	append []string		// other appends
	libmap []*Library	// step-specific libraries
	ncons int		// how many consoles allocate (hvc0 always dumps console log)
	mmegs int		// memory for step
	sysin string		// path to redirect standard input (/dev/null)
	sysout string		// path to redirect standard output (/dev/console)
	deps []string		// step target dependencies
	after []string		// wait for these steps to complete
}

func (s *Step) add_dep(dep string) () {
	s.deps = append(s.deps, dep)
}

const (
	RAW = iota		// volume raw format
	QCOW2			// volume qcow2 format
	HTTP			// http-accessible volume
	NINEP			// 9p mounted directory
	REF			// reference to other step or top library
	NOTSET			// unknown
)

