import type { Machine, MachineStatus } from "./types";

export function parseStatus(value: string): "all" | MachineStatus {
	switch (value) {
		case "all":
		case "healthy":
		case "drifted":
		case "pending":
		case "offline":
		case "blocked":
			return value;
		default:
			return "all";
	}
}

export function parseOs(value: string): "all" | Machine["os"] {
	switch (value) {
		case "all":
		case "macOS":
		case "Linux":
			return value;
		default:
			return "all";
	}
}
