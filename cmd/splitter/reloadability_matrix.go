//go:build windows

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
	"engine.stats_interval",
	"engine.policies",
	"windivert.filter (via handle reopen)",
	"windivert.queue_len (including 0/default via handle reopen)",
	"windivert.queue_time_ms (including 0/default via handle reopen)",
	"windivert.queue_size_bytes (including 0/default via handle reopen)",
}

var windowsRestartRequiredSettings = []string{
	"engine.workers",
	"windivert_dir",
	"windivert_sys",
}
