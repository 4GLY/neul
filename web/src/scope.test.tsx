import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ResourceEditor } from "./ResourceEditor";

describe("MVP scope guardrails", () => {
	it("does not expose secret or arbitrary shell editors", () => {
		const markup = renderToStaticMarkup(
			<ResourceEditor onSaved={() => undefined} />,
		).toLowerCase();

		expect(markup).not.toContain("secret");
		expect(markup).not.toContain("shell");
		expect(markup).not.toContain("websocket");
	});
});
