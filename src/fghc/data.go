// The command utility for VM job control

package main

import (
	"os/user"
)

type Job struct {
	kernel string		// which kernel to load
	kvm string		// path to KVM
	libmap []*Library	// toplevel libraries
	id string		// job ID
	idraw string		// raw job ID as provided from the command line
	idprt bool		// if true, print the job ID on the standard output as it is started (with -make -quiet)
	steps StepMap		// job steps
	mmegs int		// default memory for VM
	xdisplay int		// if positive, enable guestfwd to X server with given display
	video bool		// if true, video access (X display) is allowed
	audio bool		// if true, audio access (pulseaudio forwarding) is allowed
	wdir string		// working directory of the job
	user *user.User		// host user information
	uimp bool		// if true, import user home directory into the VM
	make bool		// if true redirect output to make -f -
	quiet bool		// if true run make quietly
	desktop string		// path to the .desktop file that started the job - only used if -user was specified
	hostname string		// try to set all VMs of this job to the given hostname
}

type Jobinfo struct {
	id string		// job identifier, as gotten from the first part of PID file name
	step string		// current step name
	kvmpid int		// PID of the KVM instance
	status string		// current status
	stalled bool		// job stalled (terminated earlier and status was not cleaned up)
	volumes []Volinfo	// currently used volumes (disk images only)
}

type Volinfo struct {
	path string		// volume path
	voltype int		// volume type, same as library type
}

type LibMapN map [int] *Library

type Library struct {
	name string		// name of the library that can be used for step reference
	stname string		// name of the step where library was defined
	path string		// host path to the library (directory or volume)
	libtype int		// type of the library (raw, qcow, 9p, reference)
	write bool		// writable
	locked bool		// write mode cannot be overridden
	id string		// id for KVM device definition
	tag string		// tag for 9p, serial for volumes
	save string		// if nonempty, save the volume at this path (ignored for 9p)
	from string		// if nonempty, copy the volume from this path into a new volume
	snap bool		// if true, use this volume in snapshot mode
	refstep string		// for references, address a step whose library is reused, or top if blank
	reflib *Library		// reference to the library in other step
	newsize int		// if positive, the library has to be created anew even if existed - volumes only
}

// Map of library prefixes is indexed by the internal mountpoint inside the VM
// and its members contain the corresponding host path and write permissions. When the
// library path mapping is on, any path provided in a fghc parameter is attempted to
// be mapped to the corresponding path on the host based on the path prefix. Posible prefixes
// are /host or user's home base found in the job user specification.

type LibPrefix map [string] struct {
	Hostpath string
	Write bool
}

type HostFwd struct {
	hport int		// host port to listen at
	gport int		// guest port to forward to
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
	host bool		// if true, it is possible to submit jobs on the host
	xkernel string		// may be set for steps that load kernel-containing images, but
				// allow host job submissions with the base kernel
	lbrst bool		// if true, library restrictions will apply to the submitted jobs
	libpfx *LibPrefix	// per-step lib prefix map which will be given to slave fghc
	hostfwd []HostFwd	// per-step host forward table
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

