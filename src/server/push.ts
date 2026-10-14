import webpush from "web-push";
import type { Store, VapidKeys } from "./db.ts";

/** Body of every push message; src/web/sw.js shows it as a notification. */
export interface PushPayload {
	title: string;
	body: string;
	url: string; // opened on click, relative to the hub origin
	tag: string; // a newer notification with the same tag replaces the older one
}

const TTL_S = 24 * 3600; // a phone that stays offline longer misses the notification

/** Sends Web Push messages with the hub's VAPID keys, created on first use (design.md §8 P2). */
export class Pusher {
	constructor(
		private readonly store: Store,
		private readonly subject: string,
	) {}

	private keys(): VapidKeys {
		const stored = this.store.getVapidKeys();
		if (stored) return stored;
		const keys = webpush.generateVAPIDKeys();
		this.store.putVapidKeys(keys);
		return keys;
	}

	publicKey(): string {
		return this.keys().publicKey;
	}

	/** Push to every subscribed device; subscriptions the push service reports as gone are removed. */
	async send(payload: PushPayload): Promise<{ sent: number; failed: number }> {
		const { publicKey, privateKey } = this.keys();
		const body = JSON.stringify(payload);
		const results = await Promise.all(
			this.store.listPushSubscriptions().map(async (sub) => {
				try {
					await webpush.sendNotification(
						{
							endpoint: sub.endpoint,
							keys: { p256dh: sub.p256dh, auth: sub.auth },
						},
						body,
						{
							TTL: TTL_S,
							vapidDetails: { subject: this.subject, publicKey, privateKey },
						},
					);
					return true;
				} catch (e) {
					const status =
						e instanceof webpush.WebPushError ? e.statusCode : null;
					if (status === 404 || status === 410) {
						this.store.deletePushSubscription(sub.endpoint);
					} else {
						console.error(`push to ${new URL(sub.endpoint).host} failed:`, e);
					}
					return false;
				}
			}),
		);
		const sent = results.filter(Boolean).length;
		return { sent, failed: results.length - sent };
	}
}
