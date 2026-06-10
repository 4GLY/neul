import type { FormEvent, ReactElement } from "react";
import { useState } from "react";
import { createPackageResource, OwnerSessionRequiredError } from "./api";
import type { ApiResource } from "./apiTypes";
import { DotfileResourceEditor } from "./DotfileResourceEditor";

type ResourceEditorProps = {
	readonly onOwnerSessionRequired?: () => void;
	readonly onSaved: () => void;
	readonly resources?: readonly ApiResource[];
};

export function ResourceEditor({
	onOwnerSessionRequired,
	onSaved,
	resources = [],
}: ResourceEditorProps): ReactElement {
	const [mode, setMode] = useState<"package" | "dotfile">("package");
	const [packageName, setPackageName] = useState("");
	const [sourceKind, setSourceKind] = useState<"brew" | "apt" | "mise">("brew");
	const [packageVersion, setPackageVersion] = useState("latest");
	const [message, setMessage] = useState("");
	const [isSaving, setIsSaving] = useState(false);

	async function handlePackageSubmit(event: FormEvent<HTMLFormElement>) {
		event.preventDefault();
		setIsSaving(true);
		setMessage("");
		try {
			await createPackageResource({
				name: packageName,
				sourceKind,
				desiredVersion: packageVersion,
				targetSegment: "base",
			});
			setMessage("저장했습니다");
			onSaved();
		} catch (error) {
			if (error instanceof OwnerSessionRequiredError) {
				onOwnerSessionRequired?.();
				return;
			}
			setMessage(
				error instanceof Error ? error.message : "저장하지 못했습니다",
			);
		} finally {
			setIsSaving(false);
		}
	}

	return (
		<section className="resource-editor">
			<header>
				<div>
					<p className="eyebrow">Desired state</p>
					<h2>리소스 편집</h2>
				</div>
				<div className="segmented">
					<button
						className={mode === "package" ? "active" : ""}
						type="button"
						onClick={() => setMode("package")}
					>
						Package
					</button>
					<button
						className={mode === "dotfile" ? "active" : ""}
						type="button"
						onClick={() => setMode("dotfile")}
					>
						Dotfile
					</button>
				</div>
			</header>
			{mode === "package" ? (
				<form className="editor-form" onSubmit={handlePackageSubmit}>
					<label>
						<span>Package name</span>
						<input
							aria-label="Package name"
							value={packageName}
							onChange={(event) => setPackageName(event.target.value)}
							onInput={(event) => setPackageName(event.currentTarget.value)}
						/>
					</label>
					<label>
						<span>Source</span>
						<select
							aria-label="Package source"
							value={sourceKind}
							onChange={(event) =>
								setSourceKind(event.target.value as "brew" | "apt" | "mise")
							}
						>
							<option value="brew">brew supported</option>
							<option value="apt">apt unsupported</option>
							<option value="mise">mise unsupported</option>
						</select>
					</label>
					<label>
						<span>Package version</span>
						<input
							aria-label="Package version"
							value={packageVersion}
							onChange={(event) => setPackageVersion(event.target.value)}
							onInput={(event) => setPackageVersion(event.currentTarget.value)}
						/>
					</label>
					<button className="primary-button" type="submit" disabled={isSaving}>
						Save package
					</button>
				</form>
			) : (
				<DotfileResourceEditor
					isSaving={isSaving}
					resources={resources}
					onMessageChange={setMessage}
					onSaved={onSaved}
					onSavingChange={setIsSaving}
					{...(onOwnerSessionRequired === undefined
						? {}
						: { onOwnerSessionRequired })}
				/>
			)}
			{message === "" ? null : <p className="editor-message">{message}</p>}
		</section>
	);
}
