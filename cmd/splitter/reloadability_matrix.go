package main

var windowsReloadableSettings = []string{
	"engine.split_mode",
	"engine.split_chunk",
	"engine.collect_timeout",
	"engine.max_buffer_bytes",
	"engine.max_held_packets",
	"engine.max_segment_payload",
	"engine.flow_idle_timeout",
	"engine.gc_interval",
	"engine.max_flows_per_worker",
	"engine.max_reassembly_bytes_per_worker",
	"engine.max_held_bytes_per_worker",
	"engine.shutdown_fail_open_timeout",
	"engine.shutdown_fail_open_max_packets",
	"engine.adapter_flush_timeout",
	"engine.policies",
	"windivert.queue_len (non-zero values)",
	"windivert.queue_time_ms (non-zero values)",
	"windivert.queue_size_bytes (non-zero values)",
}

var windowsRestartRequiredSettings = []string{
	"engine.workers",
	"engine.worker_queue_size",
	"windivert.filter",
	"windivert_dir",
	"windivert_sys",
	"windivert.queue_len=0 (revert to driver default)",
	"windivert.queue_time_ms=0 (revert to driver default)",
	"windivert.queue_size_bytes=0 (revert to driver default)",
}
