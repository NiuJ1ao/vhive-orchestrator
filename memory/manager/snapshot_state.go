package manager

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/ftrvxmtrx/fd"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"unsafe"
)

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	VMID                                         string
	VMMStatePath, GuestMemPath, InstanceSockAddr string
	BaseDir                                      string // base directory for the instance
	MetricsPath                                  string // path to csv file where the metrics should be stored
	IsRecordMode                                 bool
	GuestMemSize                                 int
	metricsModeOn                                bool
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	startAddressOnce *sync.Once // to check if start address has been initialized
	startAddress     uint64
	userFaultFD      *os.File
	trace            *Trace
	epfd             int
	quitCh           chan int

	// install the whole working set in the guest memory
	isReplayWorkingSet bool
	// prefetch the VMM state to the host memory
	isPrefetchVMMState bool
	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	isRecordDone bool
	isReplayDone bool

	isWSCopy bool

	servedNum int
	uniqueNum int

	guestMem   []byte
	workingSet []byte

	// Stats
	totalPFServed  []float64
	uniquePFServed []float64
	reusedPFServed []float64
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	s.SnapshotStateCfg = cfg

	s.trace = initTrace(s.getTraceFile())

	if s.metricsModeOn {
		s.totalPFServed = make([]float64, 0)
		s.uniquePFServed = make([]float64, 0)
		s.reusedPFServed = make([]float64, 0)
	}

	return s
}

func (s *SnapshotState) getUFFD() error {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	for {
		c, err := d.DialContext(ctx, "unix", s.InstanceSockAddr)
		if err != nil {
			if ctx.Err() != nil {
				log.Error("Failed to dial within the context timeout")
				return err
			}
			time.Sleep(1 * time.Millisecond)
			continue
		}

		defer c.Close()

		sendfdConn := c.(*net.UnixConn)

		fs, err := fd.Get(sendfdConn, 1, []string{"a file"})
		if err != nil {
			log.Error("Failed to receive the uffd")
			return err
		}

		s.userFaultFD = fs[0]

		return nil
	}
}

func (s *SnapshotState) getTraceFile() string {
	return filepath.Join(s.BaseDir, "trace")
}

func (s *SnapshotState) mapGuestMemory() error {
	fd, err := os.OpenFile(s.GuestMemPath, os.O_RDONLY, 0444)
	if err != nil {
		log.Errorf("Failed to open guest memory file: %v", err)
		return err
	}

	s.guestMem, err = unix.Mmap(int(fd.Fd()), 0, s.GuestMemSize, unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		log.Errorf("Failed to mmap guest memory file: %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) unmapGuestMemory() error {
	if err := unix.Munmap(s.guestMem); err != nil {
		log.Errorf("Failed to munmap guest memory file: %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) pollUserPageFaults(readyCh chan int) {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var (
		events [1]syscall.EpollEvent
	)

	logger.Debug("Starting polling loop")

	file, err := os.Create("/tmp/second_trace")
	if err != nil {
		log.Fatalf("Failed to open trace file for writing: %v", err)
	}
	defer file.Close()

	defer syscall.Close(s.epfd)

	readyCh <- 0

	for {
		select {
		case <-s.quitCh:
			logger.Debug("Handler received a signal to quit")
			return
		default:
			nevents, err := syscall.EpollWait(s.epfd, events[:], -1)
			if err != nil {
				logger.Fatalf("epoll_wait: %v", err)
				break
			}

			if nevents < 1 {
				panic("Wrong number of events")
			}

			for i := 0; i < nevents; i++ {
				event := events[i]

				fd := int(event.Fd)

				stateFd := int(s.userFaultFD.Fd())

				if fd != stateFd && stateFd != -1 {
					logger.Fatalf("Received event from unknown fd")
				}

				goMsg := make([]byte, sizeOfUFFDMsg())

				if nread, err := syscall.Read(fd, goMsg); err != nil || nread != len(goMsg) {
					if !errors.Is(err, syscall.EBADF) {
						log.Fatalf("Read uffd_msg failed: %v", err)
					}
					break
				}

				if event := uint8(goMsg[0]); event != uffdPageFault() {
					log.Fatal("Received wrong event type")
				}

				address := binary.LittleEndian.Uint64(goMsg[16:])

				if err := s.servePageFault(fd, address, file); err != nil {
					log.Fatalf("Failed to serve page fault")
				}
			}
		}
	}
}

func (s *SnapshotState) servePageFault(fd int, address uint64, file *os.File) error {
	s.startAddressOnce.Do(
		func() {
			s.startAddress = address
		})

	writer := csv.NewWriter(file)
	offset := address - s.startAddress

	err := writer.Write([]string{strconv.FormatUint(offset, 16)})
	if err != nil {
		log.Fatalf("Failed to write trace: %v", err)
	}
	writer.Flush()

	src := uint64(uintptr(unsafe.Pointer(&s.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(os.Getpagesize()) - 1))
	mode := uint64(0)

	if s.metricsModeOn {
		rec := Record{
			offset:    offset,
			servedNum: s.servedNum,
		}

		if s.isRecordDone {
			if !s.trace.containsRecord(rec) {
				s.uniqueNum++
			}
		} else {
			s.trace.AppendRecord(rec)
		}

		s.servedNum++
	}

	return installRegion(fd, src, dst, mode, 1)
}
