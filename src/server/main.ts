import { homedir } from "node:os";
import { join } from "node:path";
import type { Server } from "bun";
import { probe } from "../collector/run.ts";
import type {
	AddMachineRequest,
	Platform,
	Settings,
	Transport,
	UpdateMachineRequest,
} from "../shared/types.ts";
import index from "../web/index.html";
import { buildCompare } from "./compare.ts";
import { Store } from "./db.ts";
import { Hub } from "./hub.ts";

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
const hub = new Hub(store);

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

function validSettings(body: Partial<Settings>): Settings {
	const paths = body.trackedPaths;
	if (!Array.isArray(paths) || paths.some((p) => typeof p !== "string")) {
		throw new HttpError(400, "trackedPaths는 문자열 배열이어야 합니다");
	}
	const trackedPaths = paths.map((p) => p.trim()).filter(Boolean);
	if (
		trackedPaths.some((p) => p.startsWith("/") || p.split("/").includes(".."))
	) {
		throw new HttpError(
			400,
			"추적 경로는 홈 디렉터리 기준 상대 경로여야 합니다 (/, .. 불가)",
		);
	}
	const interval = body.collectIntervalMin;
	if (
		typeof interval !== "number" ||
		!Number.isInteger(interval) ||
		interval < 1 ||
		interval > 1440
	) {
		throw new HttpError(400, "수집 주기는 1~1440분 사이 정수여야 합니다");
	}
	return { trackedPaths, collectIntervalMin: interval };
}

const server = Bun.serve({
	port,
	hostname,
	development: process.env.NODE_ENV !== "production",
	// Collect requests wait for ssh (connect timeout 10s, run timeout 60s); Bun's default idle timeout is 10s.
	idleTimeout: 120,
	routes: {
		"/*": index,
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
				store.updateMachine(machine.id, {
					name,
					enabled: body.enabled,
					platform: body.platform,
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
setInterval(collectDue, 30_000);

console.log(`neul hub on ${server.url} (db ${dbPath})`);
