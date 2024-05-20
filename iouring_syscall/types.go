//go:build linux

package iouring_syscall

// syscall number
const (
	SYS_IO_URING_SETUP    = 425
	SYS_IO_URING_ENTER    = 426
	SYS_IO_URING_REGISTER = 427
)

// IOURingParams the flags, sq_thread_cpu, sq_thread_idle and WQFd fields are used to configure the io_uring instance
type IOURingParams struct {
	SQEntries    uint32 // specifies the number of submission queue entries allocated
	CQEntries    uint32 // when IORING_SETUP_CQSIZE flag is specified
	Flags        uint32 // a bit mast of 0 or more of the IORING_SETUP_*
	SQThreadCPU  uint32 // when IORING_SETUP_SQPOLL and IORING_SETUP_SQ_AFF flags are specified
	SQThreadIdle uint32 // when IORING_SETUP_SQPOLL flag is specified
	Features     uint32
	WQFd         uint32    // when IORING_SETUP_ATTACH_WQ flag is specified
	Resv         [3]uint32 // resv数组必须初始化为0

	SQOffset SubmissionQueueRingOffset
	CQOffset CompletionQueueRingOffset
}

// SubmissionQueueRingOffset describes the offsets of various ring buffer fields
type SubmissionQueueRingOffset struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Flags       uint32
	Dropped     uint32
	Array       uint32
	Resv1       uint32
	Resv2       uint64
}

// CompletionQueueRingOffset describes the offsets of various ring buffer fields
type CompletionQueueRingOffset struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Overflow    uint32
	Cqes        uint32
	Flags       uint32
	Resv1       uint32
	Resv2       uint64
}
