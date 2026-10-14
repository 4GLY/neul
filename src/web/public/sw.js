// neul service worker: shows hub pushes (src/server/push.ts PushPayload) and opens them on click.
// No caching: the hub is only useful online.

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) =>
	event.waitUntil(self.clients.claim()),
);

self.addEventListener("push", (event) => {
	const payload = event.data ? event.data.json() : {};
	event.waitUntil(
		self.registration.showNotification(payload.title ?? "늘", {
			body: payload.body ?? "",
			tag: payload.tag,
			icon: "/icon-192.png",
			badge: "/icon-192.png",
			data: { url: payload.url ?? "/#/alerts" },
		}),
	);
});

self.addEventListener("notificationclick", (event) => {
	event.notification.close();
	const url = new URL(
		event.notification.data?.url ?? "/#/alerts",
		self.location.origin,
	).href;
	event.waitUntil(
		self.clients
			.matchAll({ type: "window", includeUncontrolled: true })
			.then((windows) => {
				const open = windows.find(
					(w) => new URL(w.url).origin === self.location.origin,
				);
				if (open) return open.navigate(url).then((w) => (w ?? open).focus());
				return self.clients.openWindow(url);
			}),
	);
});
