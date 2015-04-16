// The command utility for VM job control

package main

import (
	"os"
	"fmt"
	"strings"
	"path/filepath"
	"github.com/cznic/mathutil"
	"launchpad.net/zabudka/go-src/src/smlib"
)

// Invoke KVM with all necessary options.
// All files that a VM instance creates will be added to the clean after list.
// All files will be created under the job's working directory prepending step name.
// The first virtconsole device (hvc0) will be used for dumping the console output to the file.
// Minimum 2 consoles (via sockets) will be allocated, more if ncons says so.

func dumpstep(s *Step, j *Job) {
	enkvm := " `kvm-ok >/dev/null && echo -cpu host -enable-kvm`"
	fmt.Println("\t" + j.kvm + enkvm + " -vga none -no-reboot \\")
	fmt.Println("\t -kernel " + j.kernel + " \\")
	msize := j.mmegs
	monpath := filepath.Join(j.wdir, s.name + ".monitor")
	consdump := filepath.Join(j.wdir, s.name + ".consdump")
	cleanafter = append(cleanafter, monpath)
	cleanafter = append(cleanafter, consdump)
	ncons := mathutil.Min(s.ncons, 2)
	kappend := "console=hvc0"
	encexec, e := smlib.EncJsonGzipB64(s.exec)
	if e != nil {
		fmt.Println(os.Stderr, e)
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
	fmt.Println("\t -m " + fmt.Sprint(msize) + "M \\")
	fmt.Println("\t -display none -device virtio-balloon -device virtio-serial-pci \\")
	fmt.Println("\t -chardev socket,id=mon,path=" + monpath + ",server,nowait \\")
	fmt.Println("\t -mon chardev=mon \\")
	fmt.Println("\t -chardev file,id=consdump,path=" + consdump + " \\")
	fmt.Println("\t -device virtconsole,chardev=consdump \\")
	for i := 1 ; i <= ncons ; i++ {
		conspath := filepath.Join(j.wdir, s.name + ".vcons" + fmt.Sprint(i))
		chdid := "vcons" + fmt.Sprint(i)
		cleanafter = append(cleanafter, conspath)
		fmt.Println("\t -chardev socket,id=" + chdid + ",path=" + conspath + ",server,nowait \\")
		fmt.Println("\t -device virtconsole,chardev=" + chdid + " \\")
	}
	for i, l := range s.libmap {
		prlib(i, l)
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
	fmt.Println("\t -net 'user" + red1 + red2 + "' \\")
	fmt.Println("\t -net nic,model=virtio \\")
	fmt.Println("\t -append '" + kappend + "'")
	fmt.Println("\t exit `grep ^EXITCODE: " + consdump + " | tail -n 1 | cut -d: -f 2`")
}

func prlib(i int, l *Library) {
	switch(l.libtype) {
		case REF:
			if l.reflib != nil {
				prlib(i, l.reflib)
			}
		case RAW:
			rawlib(i, l)
		case HTTP:
			httplib(i, l)
		case QCOW2:
			qcwlib(i, l)
		case NINEP:
			nplib(i, l)
	}
}

func nplib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -fsdev local" + ro + ",id=" + l.id + ",path=" + l.path + ",security_model=mapped \\")
	fmt.Println("\t -device virtio-9p-pci,fsdev=" + l.id + ",mount_tag=" + l.tag + " \\")
}

func rawlib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off,x-data-plane=on \\")
	fmt.Print("\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache=none,aio=native,format=raw" + ro + ",file=" + l.path)
	commonlib(i, l)
}

func httplib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Print("\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache=none,aio=native,format=http" + ro + ",url=" + l.path)
	commonlib(i, l)
}

func qcwlib(i int, l *Library) {
	ro := ",readonly"
	if l.write {
		ro = ""
	}
	fmt.Println("\t -device virtio-blk,drive=drive" + fmt.Sprint(i) + 
		    ",scsi=off,config-wce=off \\")
	fmt.Print("\t -drive if=none,id=drive" + fmt.Sprint(i) + 
	          ",cache=none,aio=native,format=qcow2" + ro + ",file=" + l.path)
	commonlib(i, l)
}

func commonlib(i int, l *Library) {
	if l.tag != "" {
		fmt.Print(",serial=" + l.tag)
	}
	if l.snap {
		fmt.Print(",snapshot=on")
	}
	fmt.Println(" \\")
}
