//go:build linux

package iouring_syscall

import (
	"os"
	"syscall"
	"unsafe"
)

// io_uring_setup 的flags参数
const (
	// IORING_SETUP_IOPOLL
	/*
	*	1.通过忙等待（busy-waiting）来处理I/O完成事件，而不是通过异步的中断请求。
	*	2.file system 和 block device 必须支持 polling。
	*	3.忙等待提供低延时，但是可能会比中断驱动的I/O消耗更多的CPU资源。
	*	4.目前这个特性只用于文件描述符打开时使用了 O_DIRECT 标志的文件描述符上使用。当一个读写操作被提交到轮询上下文时，应用必须调用io_uring_enter去完成事件，并让操作被返回到CQ ring上。
	*	5.把可轮询和不可轮询的I/O操作放在一个io_uring中很不公平。
	*	6.目前仅适用于存储设备，且存储设备必须配置为polling。对于NVMe设备，NVMe驱动程序必须加载poll_queues参数设置为期望的轮询队列数量。如果轮询队列的数量小于在线CPU线程的数量，则轮询队列将在系统中的CPU之间适当地共享。
	 */
	IORING_SETUP_IOPOLL uint32 = (1 << 0)

	// IORING_SETUP_SQPOLL
	/*
	*	1.当指定这个flag时，将创建一个内核线程来执行提交队列的轮询操作。
	*	2.用这种方式配置的io_uring实例使应用程序能够在不发生上下文切换的条件下去处理I/O请求。
	*	3.通过使用提交队列去提交新的请求，并且观察完成队列，应用可以在不使用系统调用提交和收割I/O任务。
	*	4.如果内核线程空闲的时间超过 sq_thread_idle 毫秒，它就会设置 struct io_sq_ring中的flags的IORING_SQ_NEED_WAKEUP的比特。当这种场景发生时，这个应用必须调用io_uring_enter去唤醒内核线程。
	*		如果I/O保持忙碌，这个内核线程永远不会休眠。应用需要使用下面的代码来保护io_uring_enter的调用：
	*
	*	  	// 确保尾指针写入后，唤醒标志被读取。对读取的标志使用内存 load acquire 语义是很重要的，否则应用程序和内核可能无法就唤醒标志达成一致。（原子操作）
	*		unsigned flags = atomic_load_relaxed(sq_ring->flags); // sq_ring是使用下面描述的io_sqring_offsets结构体设置的提交队列环。
	*		if (flags & IORING_SQ_NEED_WAKEUP)
	*			io_uring_enter(fd, 0, 0, IORING_ENTER_SQ_WAKEUP);
	*	5.当使用设置有IORING_SETUP_SQPOLL的ring时，使用者永远不会直接调用io_uring_enter系统调用。这通常由liburing库的io_uring_submit函数来处理。它会自动确定是否在使用轮询模式，当你需要使用io_uring_enter时，你也无需费心。
	 */
	IORING_SETUP_SQPOLL uint32 = (1 << 1) /* SQ poll thread */

	// IORING_SETUP_SQ_AFF
	/*
	*	1.如果指定了这个flag，这个轮询线程将会被绑定到由struct io_uring_params 的 sq_thread_cpu 字段指定的cpu集合。
	*	2.这个flag只会在IORING_SETUP_SQPOLL被指定时有效。当cgroup设置cpuset.cpus改变时（通常是在容器环境中），这个被绑定的cpu集合也会被同时改变。
	 */
	IORING_SETUP_SQ_AFF uint32 = (1 << 2) /* sq_thread_cpu is valid */

	// IORING_SETUP_CQSIZE
	/*
	*	1.创建完成队列struct io_uring_params.cq_entries entries。这个值必须大于 entries，并且可以四舍五入到下一个2的幂。
	 */
	IORING_SETUP_CQSIZE uint32 = (1 << 3) /* app defines CQ size */

	// IORING_SETUP_CLAMP
	/*
	*	1.如果指定了这个flag，当 entries 超过了 IORING_MAX_ENTRIES 时，entries 将会被降低为 IORING_MAX_ENTRIES 时。
	*	2.如果设置了 IORING_SETUP_CQSIZE，如果 struct io_uring_params.cq_entries 的值超过了 IORING_MAX_CQ_ENTRIES 时，会被降低至 IORING_MAX_CQ_ENTRIES。
	 */
	IORING_SETUP_CLAMP uint32 = (1 << 4) /* clamp SQ/CQ ring sizes */

	// IORING_SETUP_ATTACH_WQ
	/*
	*	1.这个flag应该和 struct io_uring_params.wq_fd 一起设置到一个已经存在的io_uring的fd上。
	*	2.当设置时，正在被创建的io_uring实例将会共享指定的io_uring 的异步工作线程，而不是单独创建新的线程池。
	 */
	IORING_SETUP_ATTACH_WQ uint32 = (1 << 5) /* attach to existing wq */

	// IORING_SETUP_R_DISABLED
	/*
	*	1.如果这个flag被指定了，这个io_uring将会以禁止状态启动。
	*	2.在这种状态下，restrictions 可以被注册，但是提交不被允许，详情查看 io_uring_register 怎么开启。
	 */
	IORING_SETUP_R_DISABLED uint32 = (1 << 6) /* start with ring disabled */

	// IORING_SETUP_SUBMIT_ALL
	/*
	*	1.通常，如果一批请求中的一个结果出错，io_uring将会停止提交这一批请求。如果在提交时以错误结束，这会导致提交的请求少于预期。
	*	2.如果io_uring在创建时指定了这个flag，io_uring_enter 会持续提交请求，即使它在提交时遇到错误。
	*	3.不管在io_uring创建时是否设置了这个标志，在发生错误时 CQEs 仍然会被提交。唯一的区别是，当观察到错误时提交序列是停止还是继续
	 */
	IORING_SETUP_SUBMIT_ALL uint32 = (1 << 7) /* continue submit on error */

	// IORING_SETUP_COOP_TASKRUN
	// TODO TRANSLATE
	/*
	*	1.默认情况下，当有完成的事件出现时，io_uring会打断用户空间中运行的任务。这是为了确保完成的任务及时运行。
	 */
	/*
	 * Cooperative task running. When requests complete, they often require
	 * forcing the submitter to transition to the kernel to complete. If this
	 * flag is set, work will be done when the task transitions anyway, rather
	 * than force an inter-processor interrupt reschedule. This avoids interrupting
	 * a task running in userspace, and saves an IPI.
	 */
	IORING_SETUP_COOP_TASKRUN uint32 = (1 << 8)

	// IORING_SETUP_TASKRUN_FLAG
	// TODO TRANSLATE
	/*
	*	1.
	 */
	/*
	 * If COOP_TASKRUN is set, get notified if task work is available for
	 * running and a kernel transition would be needed to run it. This sets
	 * IORING_SQ_TASKRUN in the sq ring flags. Not valid with COOP_TASKRUN.
	 */
	IORING_SETUP_TASKRUN_FLAG uint32 = (1 << 9)

	// IORING_SETUP_SQE128
	/*
	*	1.如果设置了，io_uring将会使用128字节大小的SQEs而不是普通的64字节大小的SQEs。
	*	2.这个用于特定的请求类型。只用于NVMe passthrough 的 IORING_OP_URING_CMD passthrough 的命令。
	*	3.内核5.19之后有效。
	 */
	IORING_SETUP_SQE128 uint32 = (1 << 10) /* SQEs are 128 byte */

	// IORING_SETUP_CQE32
	/*
	*	1.如果设置了，io_uring将使用32字节大小的CQE，而不是正常的16字节大小的。
	*	2.用于某些特定请求。只用于NVMe passthrough 的 IORING_OP_URING_CMD passthrough 的命令。
	*	3.内核5.19之后有效。
	 */
	IORING_SETUP_CQE32 uint32 = (1 << 11) /* CQEs are 32 byte */

	// IORING_SETUP_SINGLE_ISSUER
	/*
	*	1.只有一个线程（task）会提交请求，用于内部优化。
	*	2.提交请求的线程即是创建这个io_uring的线程，或者如果指定了 IORING_SETUP_R_DISABLED，那么它就是通过 io_uring_register 使用 io_uring 的线程。
	*	3.内核强制执行此规则，如果违反了限制，则请求失败并返回 -EEXIST。
	*	4.注意，当设置 IORING_SETUP_SQPOLL 时，它将认为轮询线程将代表用户空间执行所有提交。他总是遵守规则，无论用户空间有多少个线程执行了 io_uring_enter。
	*	5.从 Linux 6.1 起可用。
	 */
	IORING_SETUP_SINGLE_ISSUER uint32 = (1 << 12)

	// IORING_SETUP_DEFER_TASKRUN
	/*
	*	1.默认情况下，io_uring将会在任何系统调用或者线程中断结束时，处理所有的未完成工作。这可能会推迟应用程序的其它工作。
	*	2.设置了这个flag，就表示这个io_uring它应该延时工作，直到io_uring_enter调用设置了IORING_ENTER_GETEVENTS。
	*	3.设置了这个flag，io_uring 就只会在应用程序显式等待完成事件时执行 task work。
	*	4.该标志要求设置IORING_SETUP_SINGLE_ISSUER标志，并且还强制从提交请求的同一线程调用io_uring_enter(2)。请注意，如果设置了此标志，则应用程序有责任定期触发工作(例如通过任何CQE等待函数)，否则可能不会交付完成。
	*	5.从6.1开始可用。
	 */
	IORING_SETUP_DEFER_TASKRUN uint32 = (1 << 13)

	// IORING_SETUP_NO_MMAP
	/*
	*	1.默认情况下，io_uring分配内核内存，调用者必须随后使用mmap。
	*	2.如果设置了这个flag，io_uring将使用调用者分配的缓冲区；p->cq_off.user_addr 必须指向用于sq/cq环的内存，p->sq_off.user_addr必须指向sqes的内存。
	*	3.每个分配都必须是连续内存。
	*	4.通常，调用者应该通过使用mmap分配一个huge page。
	*	5.如果这个flag设置了，那么后续尝试mmap io_uring的文件描述符会将失败。
	*	6.6.5以后可用。
	 */
	IORING_SETUP_NO_MMAP uint32 = (1 << 14)

	// IORING_SETUP_REGISTERED_FD_ONLY
	/*
	*	1.如果设置了这个flag，io_uring将会注册环形fd，而且返回注册的fd的缩影，而不是返回fd。
	*	2.调用者在调用 io_uring_register 时需要使用 IORING_REGISTER_USE_REGISTERED_RING。
	*	3.这个flag只能在使用了 IORING_SETUP_NO_MMAP 时才有意义。IORING_SETUP_NO_MMAP 也需要设置。
	*	4.6.5以后可以使用。
	 */
	IORING_SETUP_REGISTERED_FD_ONLY uint32 = (1 << 15)

	// IORING_SETUP_NO_SQARRAY
	/*
	*	1.如果设置了此标志，提交队列中的entries将会按照顺序提交，到达最后一个条目后会再回到第一个条目。换句话说，将不再直接经过submission entries数组，队列将直接被 submission queue tail 索引，索引范围由它对队列大小取模表示。
	*	2.随后，用户不应该映射提交队列条目数组，并且 struct io_sqring_offsets 中的相应偏移量将被设置为0。
	*	3.6.6以后可以使用
	 */
	IORING_SETUP_NO_SQARRAY uint32 = (1 << 16)

	/*
	*	如果没有指定flags，io_uring实例将为中断驱动的I/O设置。I/O可以使用io_uring_enter提交，并且可以通过轮询completion queue来收割任务。
	*	resv数组必须初始化为0。
	 */
)

// features 由内核填充，内核指定当前内核版本的各种特性。
// io_uring features 由当前版本的内核支持。
const (
	// IORING_FEAT_SINGLE_MMAP
	/*
	*	1.如果设置了这个flag，则可以通过单个mmap系统调用映射SQ和CQ两个队列。SEQs仍然需要单独分配。
	*	2.这将mmap调用从3个减少到2个。
	*	3.5.4后可用。
	 */
	IORING_FEAT_SINGLE_MMAP uint32 = (1 << 0)

	// IORING_FEAT_NODROP
	/*
	*	1.如果设置了这个flag，iouring支持几乎不丢弃完成事件。
	*	2.丢弃事件只会发生在内核内存耗尽时，这种情况下会遇到比丢失时间更严重的问题。你的程序将会被OOM杀死。
	*	3.如果发生了完成事件并且CQ环已满，内核将在内部存储该事件，直到CQ环有空间容纳更多条目。
	*	4.在早期内核版本，如果发生了溢出，如果不能将溢出事件刷新到 CQ ring ，尝试提交更多的IO将会返回 -EBUSY 错误，如果发生这种情况应用程序必须收割CQ ring上的任务然后再次尝试提交。
	*	5.如果内核没有内存去保存事件了，则可以通过增加CQ ring上的溢出值来显示该事件。
	*	6.5.5后可用。此外 io_uring_enter(2)将返回-EBADR下一次，否则它将休眠等待 completions(自内核5.19)。
	 */
	IORING_FEAT_NODROP uint32 = (1 << 1)

	// IORING_FEAT_SUBMIT_STABLE
	/*
	*	1.如果设置了这个flag，应用程序可以确定当内核消耗SQE时，用于async offload的数据已经消耗掉了。
	*	2.5.5后可用。
	 */
	IORING_FEAT_SUBMIT_STABLE uint32 = (1 << 2)

	// IORING_FEAT_RW_CUR_POS
	/*
	*	1.如果设置了这个flag，应用可以指定 offset == -1和IORING_OP_{READV,WRITEV}，IORING_OP_{READ,WRITE}_FIXED，IORING_OP_{READ,WRITE}，这意味着当前文件的位置。这和 preadv2，pwritev2 指定offset为-1行为类似。
	*	2.它将使用（和更新）当前文件的位置。这显然伴随着一个警告，即如果应用程序在运行中有多个读或写操作，那么最终结果将不会像预期的那样。这类似于线程共享文件描述符并使用当前文件位置执行IO。
	*	3.5.6后可用。
	 */
	IORING_FEAT_RW_CUR_POS uint32 = (1 << 3)

	// IORING_FEAT_CUR_PERSONALITY
	/*
	*	If this flag is set, then io_uring guarantees that both sync and async execution of a request assumes  the  credentials
	*	of  the task that called io_uring_enter(2) to queue the requests. If this flag isn't set, then requests are issued with
	*	the credentials of the task that originally registered the io_uring. If only one task is using a ring, then  this  flag
	*	doesn't matter as the credentials will always be the same. Note that this is the default behavior, tasks can still reg‐
	*	ister different personalities through io_uring_register(2) with IORING_REGISTER_PERSONALITY and specify the personality
	*	to use in the sqe. Available since kernel 5.6.
	 */
	IORING_FEAT_CUR_PERSONALITY uint32 = (1 << 4)

	// IORING_FEAT_FAST_POLL
	/*
	*	1.如果设置了这个flag，io_uring支持使用内部轮询机制来驱动 data/space 准备情况。这意味着请求不能read和write
	 */
	IORING_FEAT_FAST_POLL uint32 = (1 << 5)

	// IORING_FEAT_POLL_32BITS
	IORING_FEAT_POLL_32BITS uint32 = (1 << 6)

	// IORING_FEAT_SQPOLL_NONFIXED
	IORING_FEAT_SQPOLL_NONFIXED uint32 = (1 << 7)

	// IORING_FEAT_ENTER_EXT_ARG
	IORING_FEAT_ENTER_EXT_ARG uint32 = (1 << 8)
)

// IOURingSetup 实现 io_uring_setup(2)
func IOURingSetup(entries uint, params *IOURingParams) (int, error) {
	res, _, errno := syscall.RawSyscall6(
		SYS_IO_URING_SETUP,
		uintptr(entries),
		uintptr(unsafe.Pointer(params)),
		0, 0, 0, 0,
	)
	if errno != 0 {
		return int(res), os.NewSyscallError("iouring_setup", errno)
	}

	return int(res), nil
}
