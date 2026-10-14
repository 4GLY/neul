import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import {
	armor,
	Decrypter,
	Encrypter,
	generateIdentity,
	identityToRecipient,
} from "age-encryption";

/**
 * age encryption with the hub's own identity, for secret file contents stored in the DB. The
 * identity file is created (mode 600) on first use and never leaves the hub: losing it makes
 * every stored secret version unreadable.
 */
export class Secrets {
	private keys: Promise<{ identity: string; recipient: string }> | null = null;

	constructor(private readonly identityPath: string) {}

	private load(): Promise<{ identity: string; recipient: string }> {
		this.keys ??= (async () => {
			let identity: string;
			if (existsSync(this.identityPath)) {
				identity = readFileSync(this.identityPath, "utf8").trim();
			} else {
				identity = await generateIdentity();
				mkdirSync(dirname(this.identityPath), { recursive: true });
				writeFileSync(this.identityPath, `${identity}\n`, {
					mode: 0o600,
					flag: "wx",
				});
			}
			return { identity, recipient: await identityToRecipient(identity) };
		})();
		// A failed load (e.g. unwritable directory) should be retried on the next call.
		this.keys.catch(() => {
			this.keys = null;
		});
		return this.keys;
	}

	/** Armored age ciphertext of `text`. */
	async encrypt(text: string): Promise<string> {
		const { recipient } = await this.load();
		const encrypter = new Encrypter();
		encrypter.addRecipient(recipient);
		return armor.encode(await encrypter.encrypt(text));
	}

	async decrypt(armored: string): Promise<string> {
		const { identity } = await this.load();
		const decrypter = new Decrypter();
		decrypter.addIdentity(identity);
		return decrypter.decrypt(armor.decode(armored), "text");
	}
}
