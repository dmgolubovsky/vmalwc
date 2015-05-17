// The command utility for VM job control

package main

import (
	"io"
	"os"
	"fmt"
	"strings"
	"path/filepath"
	"github.com/bu-/magic"
	"github.com/cznic/mathutil"
	"launchpad.net/zabudka/go-src/src/smlib"
)

// Invoke KVM with all necessary options.
// All files that a VM instance creates will be added to the clean after list.
// All files will be created under the job's working directory prepending step name.
// The first virtconsole device (hvc0) will be used for dumping the console output to the file.
// Minimum 2 consoles (via sockets) will be allocated, more if ncons says so.

func dumpstep(p io.WriteCloser, s *Step, j *Job) {
	rstep := j.id + "." + s.name
	status := filepath.Join(j.wdir, rstep + ".status")
	fmt.Fprintln(p, "\t echo 'in progress' >" + status)
	enkvm := " `kvm-ok >/dev/null && echo -cpu host -enable-kvm`"
	fmt.Fprintln(p, "\t(" + j.kvm + enkvm + " -vga none -no-reboot \\")
	fmt.Fprintln(p, "\t -kernel " + j.kernel + " \\")
	msize := j.mmegs
	pidfile := filepath.Join(j.wdir, rstep + ".pid")
	monpath := filepath.Join(j.wdir, rstep + ".monitor")
	consdump := filepath.Join(j.wdir, rstep + ".consdump")
	cleanafter = append(cleanafter, monpath)
	cleanafter = append(cleanafter, consdump)
	cleanafter = append(cleanafter, pidfile)
	cleanafter = append(cleanafter, status)
	ncons := mathutil.Min(s.ncons, 2)
	kappend := "console=hvc0 step=" + rstep + " jobid=" + j.id
	if len(j.hostname) > 0 {
		kappend = kappend + " hostname=" + j.hostname
	}
	encexec, e := smlib.EncJsonGzipB64(s.exec)
	if e != nil {
		fmt.Fprintln(p, os.Stderr, e)
		os.Exit(1)
	}
	if len(s.exec) > 0 {
		kappend = kappend + " exec=" + encexec
	} else {
		kappend = kappend + " exec=none"
	}
	if len(s.sysin) > 0 {
		kappend = kappend + " sysin=" + s.sysin
	}
	if len(s.sysout) > 0 {
		kappend = kappend + " sysout=" + s.sysout
	}
	if s.mmegs > 0 {
		msize = s.mmegs
	}
	fmt.Fprintln(p, "\t -m " + fmt.Sprint(msize) + "M \\")
	fmt.Fprintln(p, "\t -pidfile " + pidfile + " \\")
	fmt.Fprintln(p, "\t -display none -device virtio-balloon -device virtio-serial-pci \\")
	fmt.Fprintln(p, "\t -chardev socket,id=mon,path=" + monpath + ",server,nowait \\")
	fmt.Fprintln(p, "\t -mon chardev=mon \\")
	fmt.Fprintln(p, "\t -chardev file,id=consdump,path=" + consdump + " \\")
	fmt.Fprintln(p, "\t -device virtconsole,chardev=consdump \\")
	for i := 1 ; i <= ncons ; i++ {
		conspath := filepath.Join(j.wdir, rstep + ".vcons" + fmt.Sprint(i))
		chdid := "vcons" + fmt.Sprint(i)
		cleanafter = append(cleanafter, conspath)
		fmt.Fprintln(p, "\t -chardev socket,id=" + chdid + ",path=" + conspath + ",server,nowait \\")
		fmt.Fprintln(p, "\t -device virtconsole,chardev=" + chdid + " \\")
	}
	for i, l := range s.libmap {
		prlib(p, i, l)
	}
	if j.uimp {
		var usrname = ""
		if len(j.user.Name) > 0 {
			usrname = j.user.Name
		} else {
			usrname = "host_user"
		}
		kappend = kappend + " user=" + usrname
		kappend = kappend + " uid=" + j.user.Uid
		kappend = kappend + " gid=" + j.user.Gid
		kappend = kappend + " homebase=" + j.user.HomeDir
	}
	red1 := ""
	red2 := ""
	red3 := ""
	if j.video && (j.xdisplay >= 0) {
		red1 = ",guestfwd=tcp:10.0.2.100:6000-cmd:socat stdio unix-connect:/tmp/.X11-unix/X" + fmt.Sprint(j.xdisplay)
		kappend = kappend + " display=10.0.2.100:0"
	}
	paudio := os.Getenv("PULSE_SERVER")
	xaudio := os.Getenv("PULSE_EXTERNAL_SERVER")
	if j.audio && (len(paudio) > 0) {
		kappend = kappend + " pulse=tcp:10.0.2.200:4713"
		papts := strings.Split(paudio, "}")
		pasrv := ""
		if len(papts) == 2 {
			pasrv = papts[1]
		} else
		{
			pasrv = paudio
	
		}
		red2 = ",guestfwd=tcp:10.0.2.200:4713-cmd:socat stdio " + pasrv
	} else if j.audio && (len(xaudio) > 0) {
		kappend = kappend + " pulse=tcp:10.0.2.200:4713"
		red2 = ",guestfwd=tcp:10.0.2.200:4713-cmd:socat stdio " + xaudio
	}
	if s.host {
		fenv := "env PULSE_SERVER=" + paudio + " PULSE_EXTERNAL_SERVER=" + xaudio + " "
		fghc := os.Args[0] + " -kernel " + j.kernel + " -kvm " + j.kvm + 
			" -mem " + fmt.Sprint(j.mmegs) + "M" + " -workdir " + j.wdir
		red3 = ",guestfwd=tcp:10.0.2.150:77-cmd:xargs " + fenv + fghc
		kappend = kappend + " hostdisplay=" + fmt.Sprint(j.xdisplay)
	}
	fmt.Fprintln(p, "\t -net 'user" + red1 + red2 + red3 + "' \\")
	fmt.Fprintln(p, "\t -net nic,model=virtio \\")
	fmt.Fprintln(p, "\t -append \"" + kappend + "\" ; exit `echo $$? | tee " + status + "` ) && \\")
	fmt.Fprintln(p, "\t\t exit `grep ^EXITCODE: " + consdump + " | tail -n 1 | cut -d: -f 2 | tee " + status + "`")
}

func prlib(p io.WriteCloser, i int, l *Library) {
	switch(l.libtype) {
		case NOTSET:
			mgc, e := magic.Open(0)
			if e != nil {
				return
			}
			defer mgc.Close()
			e = mgc.Load("/usr/share/misc/magic.mgc")
			if e != nil {
				return
			}
			mgs, e := mgc.File(l.path)
			if e != nil {
				return
			}
			vt := guessvol(mgs)
			if vt == NOTSET {
				return
			}
			l.libtype = vt
			prlib(p, i, l)
		case REF:
			if l.reflib != nil {
				prlib(p, i, l.reflib)
			}
		case RAW:
			rawlib(p, i, l)
		case HTTP:
			httplib(p, i, l)
		case QCOW2:
			qcwlib(p, i, l)
		case NINEP:
			nplib(p, i, l)
	}
}

func nplib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -fsdev local" + ro + ",id=" + l.id + ",path=" + l.path + ",security_model=mapped \\")
	fmt.Fprintln(p, "\t -device virtio-9p-pci,fsdev=" + l.id + ",mount_tag=" + l.tag + " \\")
}

func rawlib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off,x-data-plane=on \\")
	fmt.Fprint(p, "\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache.direct=on,aio=native,format=raw" + ro + ",file=" + l.path)
	commonlib(p, i, l)
}

func httplib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Fprint(p, "\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache.direct=on,aio=native,format=http" + ro + ",url=" + l.path)
	commonlib(p, i, l)
}

func qcwlib(p io.WriteCloser, i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Fprintln(p, "\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Fprint(p, "\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache.direct=on,aio=native,format=qcow2" + ro + ",file=" + l.path)
	commonlib(p, i, l)
}

func commonlib(p io.WriteCloser, i int, l *Library) {
	if l.tag != "" {
		fmt.Fprint(p, ",serial=" + l.tag)
	}
	if l.snap {
		fmt.Fprint(p, ",snapshot=on")
	}
	fmt.Fprintln(p, " \\")
}
