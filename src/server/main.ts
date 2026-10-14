import { homedir } from "node:os";
import { join } from "node:path";
import type { Server } from "bun";
import { probe } from "../collector/run.ts";
import type {
	AddMachineRequest,
	AlertRules,
	ApplyRequest,
	DesiredInput,
	DesiredManager,
	DesiredUpdate,
	ExecEvent,
	ExecRequest,
	Platform,
	PushSubscriptionInput,
	SavedCommandInput,
	Settings,
	Transport,
	UpdateMachineRequest,
} from "../shared/types.ts";
import index from "../web/index.html";
import { buildCompare } from "./compare.ts";
import { Store } from "./db.ts";
import {
	blockedReason,
	Desired,
	NAME_PATTERN,
	VERSION_PATTERN,
} from "./desired.ts";
import {
	EXEC_DEFAULT_TIMEOUT_S,
	EXEC_MAX_TIMEOUT_S,
	Executor,
} from "./exec.ts";
import { Hub } from "./hub.ts";
import { Monitor } from "./monitor.ts";
import { Pusher } from "./push.ts";

const port = Number(process.env.NEUL_PORT ?? 7430);
const hostname = process.env.NEUL_HOST ?? "127.0.0.1";
const dbPath =
	process.env.NEUL_DB ?? join(homedir(), ".local/share/neul/neul.db");
const allowedLogins = new Set(
	(process.env.NEUL_ALLOWED_LOGINS ?? "")
		.split(",")
		.map((s) => s.trim())
		.filter(Boolean),
);

const store = new Store(dbPath);
// Push services require a contact in the VAPID claims; the hub owner's login is the natural one.
const contact = [...allowedLogins].find((login) => login.includes("@"));
const pusher = new Pusher(
	store,
	process.env.NEUL_VAPID_SUBJECT ??
		(contact ? `mailto:${contact}` : "https://github.com/4GLY/neul"),
);
const monitor = new Monitor(store, pusher);
const hub = new Hub(store, (...args) => monitor.afterRun(...args));
// Apply targets re-collect their machine as soon as they finish (design.md §8 P3).
const executor = new Executor(store, (run, machine) => {
	if (run.kind === "apply") desired.afterApply(machine);
});
const desired = new Desired(store, hub, executor);
const interrupted = store.interruptExecRuns(new Date().toISOString());
if (interrupted)
	console.log(
		`marked ${interrupted} exec target(s) left running as interrupted`,
	);

class HttpError extends Error {
	constructor(
		readonly status: number,
		message: string,
	) {
		super(message);
	}
}

const LOOPBACK: Record<string, true> = {
	"127.0.0.1": true,
	"::1": true,
	"::ffff:127.0.0.1": true,
};

/**
 * tailscale serve forwards tailnet requests from loopback and sets Tailscale-User-Login;
 * requests without it must come from the hub machine itself (design.md §3).
 */
function authenticate(req: Request, server: Server<undefined>): string {
	const login = req.headers.get("Tailscale-User-Login");
	if (login) {
		if (!allowedLogins.has(login))
			throw new HttpError(403, `${login}은(는) 허용된 사용자가 아닙니다`);
		return login;
	}
	const ip = server.requestIP(req)?.address ?? "";
	if (!LOOPBACK[ip])
		throw new HttpError(401, "tailscale serve를 통해 접속하세요");
	return "local";
}

type Handler<P extends string> = (
	req: Bun.BunRequest<P>,
	server: Server<undefined>,
) => unknown | Promise<unknown>;

/** JSON API wrapper: authentication, error mapping, JSON encoding. */
function api<P extends string>(handler: Handler<P>) {
	return async (
		req: Bun.BunRequest<P>,
		server: Server<undefined>,
	): Promise<Response> => {
		try {
			authenticate(req, server);
			const body = await handler(req, server);
			return body instanceof Response ? body : Response.json(body ?? null);
		} catch (e) {
			if (e instanceof HttpError)
				return Response.json({ error: e.message }, { status: e.status });
			console.error(e);
			return Response.json(
				{ error: e instanceof Error ? e.message : String(e) },
				{ status: 500 },
			);
		}
	};
}

async function readJson<T>(req: Request): Promise<T> {
	try {
		return (await req.json()) as T;
	} catch {
		throw new HttpError(400, "요청 본문이 올바른 JSON이 아닙니다");
	}
}

function machineParam(req: { params: { id: string } }) {
	const machine = store.getMachine(Number(req.params.id));
	if (!machine) throw new HttpError(404, "머신을 찾을 수 없습니다");
	return machine;
}

const TRANSPORTS: Record<Transport, true> = { local: true, ssh: true };
const PLATFORMS: Record<Platform, true> = { posix: true, windows: true };

async function addMachine(req: Request) {
	const body = await readJson<AddMachineRequest>(req);
	const alias = body.alias?.trim();
	if (!alias) throw new HttpError(400, "SSH 별칭을 입력하세요");
	const transport = body.transport ?? "ssh";
	if (!TRANSPORTS[transport])
		throw new HttpError(400, `알 수 없는 transport: ${transport}`);
	if (body.platform && !PLATFORMS[body.platform])
		throw new HttpError(400, `알 수 없는 platform: ${body.platform}`);
	const name = body.name?.trim() || alias;
	if (store.listMachines().some((m) => m.name === name)) {
		throw new HttpError(400, `이미 같은 이름의 머신이 있습니다: ${name}`);
	}
	let platform: Platform = body.platform ?? "posix";
	if (transport === "ssh" && !body.platform) {
		try {
			platform = await probe(alias);
		} catch (e) {
			throw new HttpError(400, e instanceof Error ? e.message : String(e));
		}
	}
	const machine = store.addMachine({ name, alias, transport, platform });
	await hub.collect(machine);
	return hub.summary(machine);
}

function relativePaths(value: unknown, label: string): string[] {
	if (!Array.isArray(value) || value.some((p) => typeof p !== "string")) {
		throw new HttpError(400, `${label}는 문자열 배열이어야 합니다`);
	}
	const paths = value.map((p: string) => p.trim()).filter(Boolean);
	if (paths.some((p) => p.startsWith("/") || p.split("/").includes(".."))) {
		throw new HttpError(
			400,
			`${label}는 홈 디렉터리 기준 상대 경로여야 합니다 (/, .. 불가)`,
		);
	}
	return paths;
}

function threshold(
	value: unknown,
	label: string,
	min: number,
	max: number,
): number | null {
	if (value === null) return null;
	if (
		typeof value !== "number" ||
		!Number.isInteger(value) ||
		value < min ||
		value > max
	) {
		throw new HttpError(
			400,
			`${label}은(는) ${min}~${max} 사이 정수이거나 꺼져 있어야 합니다`,
		);
	}
	return value;
}

function validAlertRules(body: Partial<AlertRules> | undefined): AlertRules {
	if (!body || typeof body !== "object")
		throw new HttpError(400, "alertRules가 필요합니다");
	if (typeof body.trackedFiles !== "boolean")
		throw new HttpError(400, "trackedFiles는 true/false여야 합니다");
	return {
		unreachableMin: threshold(body.unreachableMin, "연결 끊김 기준", 1, 10080),
		diskPercent: threshold(body.diskPercent, "디스크 기준", 1, 100),
		updatesCount: threshold(body.updatesCount, "업데이트 기준", 1, 10000),
		trackedFiles: body.trackedFiles,
		repos: relativePaths(body.repos, "저장소 경로"),
	};
}

function validSettings(body: Partial<Settings>): Settings {
	const trackedPaths = relativePaths(body.trackedPaths, "추적 경로");
	const interval = body.collectIntervalMin;
	if (
		typeof interval !== "number" ||
		!Number.isInteger(interval) ||
		interval < 1 ||
		interval > 1440
	) {
		throw new HttpError(400, "수집 주기는 1~1440분 사이 정수여야 합니다");
	}
	return {
		trackedPaths,
		collectIntervalMin: interval,
		alertRules: validAlertRules(body.alertRules),
	};
}

const DESIRED_MANAGERS: Record<DesiredManager, true> = {
	brew_formula: true,
	brew_cask: true,
	pacman: true,
	mise: true,
};

// Package installs can download a lot; apply targets get far longer than an ad hoc command.
const APPLY_TIMEOUT_S = 1800;

function validManager(value: unknown): DesiredManager {
	if (typeof value !== "string" || !(value in DESIRED_MANAGERS))
		throw new HttpError(400, `알 수 없는 관리 대상입니다: ${String(value)}`);
	return value as DesiredManager;
}

/**
 * Version, ensure and targets of a desired item. Targets must already be collected and able to
 * apply the manager, so a desired state that can never be applied is never stored (design.md §1).
 */
function validDesiredFields(
	manager: DesiredManager,
	name: string,
	body: { version?: unknown; ensure?: unknown; machineIds?: unknown },
) {
	const ensure = body.ensure;
	if (ensure !== "present" && ensure !== "absent")
		throw new HttpError(400, "ensure는 present 또는 absent여야 합니다");
	let version: string | null = null;
	if (manager === "mise") {
		const raw = typeof body.version === "string" ? body.version.trim() : "";
		if (raw && !VERSION_PATTERN.test(raw))
			throw new HttpError(400, `올바르지 않은 버전입니다: ${raw}`);
		if (ensure === "present" && !raw)
			throw new HttpError(
				400,
				"mise 도구는 버전을 정해야 합니다 (예: latest, 22)",
			);
		version = raw || null;
	} else if (
		body.version !== undefined &&
		body.version !== null &&
		body.version !== ""
	) {
		throw new HttpError(400, "brew·pacman 패키지는 버전을 고정할 수 없습니다");
	}
	const ids = body.machineIds;
	if (
		!Array.isArray(ids) ||
		ids.length === 0 ||
		ids.some((id) => !Number.isInteger(id))
	)
		throw new HttpError(400, "적용할 머신을 하나 이상 고르세요");
	if (new Set(ids).size !== ids.length)
		throw new HttpError(400, "같은 머신이 두 번 들어 있습니다");
	for (const id of ids as number[]) {
		const machine = store.getMachine(id);
		if (!machine) throw new HttpError(400, `없는 머신입니다: ${id}`);
		const inventory = store.latestInventory(id)?.inventory ?? null;
		const reason =
			blockedReason(manager, machine, inventory) ??
			(inventory ? null : "아직 수집되지 않았습니다");
		if (reason)
			throw new HttpError(
				400,
				`${machine.name}에 ${name}을(를) 적용할 수 없습니다: ${reason}`,
			);
	}
	return { version, ensure, machineIds: ids as number[] } as const;
}

function validSubscription(body: Partial<PushSubscriptionInput>) {
	const { endpoint, keys } = body;
	if (
		typeof endpoint !== "string" ||
		!endpoint.startsWith("https://") ||
		typeof keys?.p256dh !== "string" ||
		typeof keys.auth !== "string"
	) {
		throw new HttpError(400, "푸시 구독 정보가 올바르지 않습니다");
	}
	return { endpoint, keys: { p256dh: keys.p256dh, auth: keys.auth } };
}

const PUBLIC_DIR = new URL("../web/public/", import.meta.url);

/**
 * Files that need fixed root paths: the service worker's scope is its own path, and the manifest
 * (bundled from index.html) refers to the icons by absolute URL.
 */
function publicFile(name: string, type: string) {
	const file = Bun.file(new URL(name, PUBLIC_DIR));
	return () =>
		new Response(file, {
			headers: { "Content-Type": type, "Cache-Control": "no-cache" },
		});
}

const MAX_COMMAND_LENGTH = 64 * 1024;

function validCommand(command: unknown): string {
	if (typeof command !== "string" || command.trim() === "")
		throw new HttpError(400, "명령을 입력하세요");
	if (command.length > MAX_COMMAND_LENGTH)
		throw new HttpError(400, "명령이 너무 깁니다 (64 KiB 이하)");
	return command;
}

async function startExec(req: Request, server: Server<undefined>) {
	const login = authenticate(req, server);
	const body = await readJson<Partial<ExecRequest>>(req);
	const command = validCommand(body.command);
	const ids = body.machineIds;
	if (
		!Array.isArray(ids) ||
		ids.length === 0 ||
		ids.some((id) => !Number.isInteger(id))
	) {
		throw new HttpError(400, "실행할 머신을 하나 이상 고르세요");
	}
	if (new Set(ids).size !== ids.length)
		throw new HttpError(400, "같은 머신이 두 번 들어 있습니다");
	const timeoutS = body.timeoutS ?? EXEC_DEFAULT_TIMEOUT_S;
	if (
		!Number.isInteger(timeoutS) ||
		timeoutS < 1 ||
		timeoutS > EXEC_MAX_TIMEOUT_S
	) {
		throw new HttpError(
			400,
			`제한 시간은 1~${EXEC_MAX_TIMEOUT_S}초 사이 정수여야 합니다`,
		);
	}
	const machines = ids.map((id) => {
		const machine = store.getMachine(id);
		if (!machine) throw new HttpError(400, `없는 머신입니다: ${id}`);
		return machine;
	});
	const busy = executor.busyMachines(ids);
	if (busy.length > 0) {
		const names = machines
			.filter((m) => busy.includes(m.id))
			.map((m) => m.name);
		throw new HttpError(
			409,
			`이미 명령을 실행 중인 머신이 있습니다: ${names.join(", ")}`,
		);
	}
	const run = executor.start({
		kind: "command",
		command,
		timeoutS,
		createdBy: login,
		targets: machines.map((machine) => ({ machine, command: null })),
	});
	return { id: run.id };
}

function execRunParam(req: { params: { id: string } }) {
	const run = executor.snapshot(Number(req.params.id));
	if (!run) throw new HttpError(404, "실행 기록을 찾을 수 없습니다");
	return run;
}

const SSE_KEEPALIVE_MS = 15_000;

/** Snapshot first, then live events until `done` (design.md §6). */
function execEvents(
	req: Bun.BunRequest<"/api/exec/:id/events">,
	server: Server<undefined>,
) {
	const id = Number(req.params.id);
	execRunParam(req);
	server.timeout(req, 0); // the stream outlives Bun.serve's idleTimeout
	const encoder = new TextEncoder();
	let cleanup = () => {};
	const stream = new ReadableStream<Uint8Array>({
		start(controller) {
			// A client that went away must not break the executor's other listeners.
			const write = (text: string) => {
				try {
					controller.enqueue(encoder.encode(text));
				} catch {
					cleanup();
				}
			};
			const send = (event: ExecEvent) =>
				write(`event: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`);
			// Snapshot and subscription happen in one synchronous step, so no event is lost between them.
			const run = executor.snapshot(id);
			if (!run) return controller.close();
			send({ type: "snapshot", run });
			const unsubscribe = executor.subscribe(id, (event) => {
				send(event);
				if (event.type === "done") {
					cleanup();
					controller.close();
				}
			});
			if (!unsubscribe) {
				send({ type: "done", finishedAt: run.finishedAt ?? run.createdAt });
				return controller.close();
			}
			const keepalive = setInterval(
				() => write(": keepalive\n\n"),
				SSE_KEEPALIVE_MS,
			);
			cleanup = () => {
				clearInterval(keepalive);
				unsubscribe();
			};
		},
		cancel() {
			cleanup();
		},
	});
	return new Response(stream, {
		headers: {
			"Content-Type": "text/event-stream",
			"Cache-Control": "no-cache",
		},
	});
}

function validSavedCommand(body: Partial<SavedCommandInput>, id?: number) {
	const name = typeof body.name === "string" ? body.name.trim() : "";
	if (!name) throw new HttpError(400, "이름을 입력하세요");
	if (name.length > 100)
		throw new HttpError(400, "이름은 100자 이하여야 합니다");
	if (store.listSavedCommands().some((c) => c.name === name && c.id !== id))
		throw new HttpError(400, `이미 같은 이름의 명령이 있습니다: ${name}`);
	return { name, command: validCommand(body.command) };
}

function savedCommandParam(req: { params: { id: string } }) {
	const command = store.getSavedCommand(Number(req.params.id));
	if (!command) throw new HttpError(404, "저장된 명령을 찾을 수 없습니다");
	return command;
}

const server = Bun.serve({
	port,
	hostname,
	development: process.env.NODE_ENV !== "production",
	// Collect requests wait for ssh (connect timeout 10s, run timeout 60s); Bun's default idle timeout is 10s.
	idleTimeout: 120,
	routes: {
		"/*": index,
		"/sw.js": publicFile("sw.js", "text/javascript; charset=utf-8"),
		"/icon-192.png": publicFile("icon-192.png", "image/png"),
		"/icon-512.png": publicFile("icon-512.png", "image/png"),
		"/api/*": api(() => {
			throw new HttpError(404, "없는 API입니다");
		}),
		"/api/me": api((req, srv) => ({ login: authenticate(req, srv) })),
		"/api/machines": {
			GET: api(() => store.listMachines().map((m) => hub.summary(m))),
			POST: api((req) => addMachine(req)),
		},
		"/api/machines/:id": {
			GET: api((req) => hub.detail(machineParam(req))),
			PATCH: api(async (req) => {
				const machine = machineParam(req);
				const body = await readJson<UpdateMachineRequest>(req);
				if (body.platform && !PLATFORMS[body.platform]) {
					throw new HttpError(400, `알 수 없는 platform: ${body.platform}`);
				}
				const name = body.name?.trim();
				if (name !== undefined && name === "")
					throw new HttpError(400, "이름은 비워 둘 수 없습니다");
				if (
					name &&
					store
						.listMachines()
						.some((m) => m.name === name && m.id !== machine.id)
				) {
					throw new HttpError(400, `이미 같은 이름의 머신이 있습니다: ${name}`);
				}
				if (
					body.notifyUnreachable !== undefined &&
					typeof body.notifyUnreachable !== "boolean"
				)
					throw new HttpError(400, "notifyUnreachable은 true/false여야 합니다");
				store.updateMachine(machine.id, {
					name,
					enabled: body.enabled,
					platform: body.platform,
					notifyUnreachable: body.notifyUnreachable,
				});
				return hub.summary(machineParam(req));
			}),
			DELETE: api((req) => {
				store.deleteMachine(machineParam(req).id);
				return { ok: true };
			}),
		},
		"/api/machines/:id/collect": {
			POST: api((req) => hub.collect(machineParam(req))),
		},
		"/api/collect": {
			POST: api(() => hub.collectAll()),
		},
		"/api/compare": api(() =>
			buildCompare(
				store
					.listMachines()
					.filter((m) => m.enabled)
					.map((m) => ({
						...hub.summary(m),
						inventory: store.latestInventory(m.id)?.inventory ?? null,
					})),
			),
		),
		"/api/settings": {
			GET: api(() => store.getSettings()),
			PUT: api(async (req) => {
				const settings = validSettings(await readJson<Partial<Settings>>(req));
				store.putSettings(settings);
				return settings;
			}),
		},
		"/api/exec": {
			GET: api(() => store.listExecRuns(100)),
			POST: api((req, srv) => startExec(req, srv)),
		},
		"/api/exec/:id": api((req) => execRunParam(req)),
		"/api/exec/:id/events": api((req, srv) => execEvents(req, srv)),
		"/api/exec/:id/cancel": {
			POST: api((req) => {
				execRunParam(req);
				if (!executor.cancel(Number(req.params.id)))
					throw new HttpError(409, "이미 끝난 실행입니다");
				return { ok: true };
			}),
		},
		"/api/commands": {
			GET: api(() => store.listSavedCommands()),
			POST: api(async (req) => {
				const { name, command } = validSavedCommand(
					await readJson<Partial<SavedCommandInput>>(req),
				);
				return store.addSavedCommand(name, command);
			}),
		},
		"/api/desired": {
			GET: api(() => desired.view()),
			POST: api(async (req) => {
				const body = await readJson<Partial<DesiredInput>>(req);
				const manager = validManager(body.manager);
				const name = typeof body.name === "string" ? body.name.trim() : "";
				if (!NAME_PATTERN.test(name))
					throw new HttpError(400, `올바르지 않은 이름입니다: ${name}`);
				if (
					store
						.listDesired()
						.some((d) => d.manager === manager && d.name === name)
				)
					throw new HttpError(400, `이미 관리 중인 항목입니다: ${name}`);
				return store.addDesired({
					manager,
					name,
					...validDesiredFields(manager, name, body),
				});
			}),
		},
		"/api/desired/candidates": api((req) => {
			const params = new URL(req.url).searchParams;
			const name = params.get("name") ?? "";
			if (!NAME_PATTERN.test(name))
				throw new HttpError(400, `올바르지 않은 이름입니다: ${name}`);
			return desired.candidates(validManager(params.get("manager")), name);
		}),
		"/api/desired/plan": api(() => desired.plans()),
		"/api/desired/apply": {
			POST: api(async (req, srv) => {
				const login = authenticate(req, srv);
				const body = await readJson<Partial<ApplyRequest>>(req);
				if (!Array.isArray(body.plans) || body.plans.length === 0)
					throw new HttpError(400, "적용할 머신을 하나 이상 고르세요");
				const current = new Map(desired.plans().map((p) => [p.machineId, p]));
				const confirmed = body.plans.map(({ machineId, command }) => {
					const plan = current.get(machineId);
					if (!plan || plan.command !== command)
						throw new HttpError(
							409,
							"계획이 바뀌었습니다. 새 계획을 확인한 뒤 다시 적용하세요",
						);
					const machine = store.getMachine(machineId);
					if (!machine)
						throw new HttpError(400, `없는 머신입니다: ${machineId}`);
					return { plan, machine };
				});
				const busy = executor.busyMachines(confirmed.map((c) => c.machine.id));
				if (busy.length > 0)
					throw new HttpError(
						409,
						"명령을 실행 중인 머신이 있습니다. 끝난 뒤 다시 적용하세요",
					);
				const run = executor.start({
					kind: "apply",
					command: `맞추기 적용 · ${confirmed
						.map(({ plan }) => `${plan.machineName} ${plan.steps.length}건`)
						.join(", ")}`,
					timeoutS: APPLY_TIMEOUT_S,
					createdBy: login,
					targets: confirmed.map(({ plan, machine }) => ({
						machine,
						command: plan.command,
					})),
				});
				return { id: run.id };
			}),
		},
		"/api/desired/:id": {
			PATCH: api(async (req) => {
				const item = store.getDesired(Number(req.params.id));
				if (!item) throw new HttpError(404, "관리 항목을 찾을 수 없습니다");
				const body = await readJson<DesiredUpdate>(req);
				store.updateDesired(
					item.id,
					validDesiredFields(item.manager, item.name, {
						version: body.version === undefined ? item.version : body.version,
						ensure: body.ensure ?? item.ensure,
						machineIds: body.machineIds ?? item.machineIds,
					}),
				);
				return store.getDesired(item.id);
			}),
			DELETE: api((req) => {
				store.deleteDesired(Number(req.params.id));
				return { ok: true };
			}),
		},
		"/api/alerts": api((req) => {
			const state = new URL(req.url).searchParams.get("state") ?? "open";
			if (state !== "open" && state !== "all")
				throw new HttpError(400, "state는 open 또는 all이어야 합니다");
			return store.listAlerts(state === "all");
		}),
		"/api/push/key": api(() => ({ publicKey: pusher.publicKey() })),
		"/api/push/subscriptions": {
			POST: api(async (req) => {
				const sub = validSubscription(
					await readJson<Partial<PushSubscriptionInput>>(req),
				);
				store.putPushSubscription(sub, req.headers.get("User-Agent"));
				return { ok: true };
			}),
			DELETE: api(async (req) => {
				const { endpoint } = await readJson<{ endpoint?: unknown }>(req);
				if (typeof endpoint !== "string")
					throw new HttpError(400, "endpoint가 필요합니다");
				store.deletePushSubscription(endpoint);
				return { ok: true };
			}),
		},
		"/api/push/test": {
			POST: api(() =>
				pusher.send({
					title: "늘 · 테스트 알림",
					body: "이 기기에서 허브 알림을 받을 수 있습니다.",
					url: "/#/alerts",
					tag: "test",
				}),
			),
		},
		"/api/commands/:id": {
			PATCH: api(async (req) => {
				const saved = savedCommandParam(req);
				const { name, command } = validSavedCommand(
					await readJson<Partial<SavedCommandInput>>(req),
					saved.id,
				);
				return store.updateSavedCommand(saved.id, name, command);
			}),
			DELETE: api((req) => {
				store.deleteSavedCommand(savedCommandParam(req).id);
				return { ok: true };
			}),
		},
	},
});

/** Collect every enabled machine whose last run is older than the configured interval. */
function collectDue(): void {
	const intervalMs = store.getSettings().collectIntervalMin * 60_000;
	const now = Date.now();
	for (const machine of store.listMachines()) {
		if (!machine.enabled || hub.isCollecting(machine.id)) continue;
		const last = store.lastRun(machine.id);
		if (last && now - Date.parse(last.finishedAt) < intervalMs) continue;
		hub
			.collect(machine)
			.catch((e) => console.error(`collect ${machine.name}:`, e));
	}
}

collectDue();
monitor.tick(Date.now());
setInterval(() => {
	collectDue();
	monitor.tick(Date.now());
}, 30_000);

console.log(`neul hub on ${server.url} (db ${dbPath})`);
