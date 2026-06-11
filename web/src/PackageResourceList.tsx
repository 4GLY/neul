import type { ReactElement } from "react";
import type { ResourceRow } from "./types";

type PackageResourceListProps = {
	readonly isSaving: boolean;
	readonly resources: readonly ResourceRow[];
	readonly onDelete: (resourceId: string) => void;
	readonly onEdit: (resource: ResourceRow) => void;
};

export function PackageResourceList({
	isSaving,
	resources,
	onDelete,
	onEdit,
}: PackageResourceListProps): ReactElement | null {
	const packages = resources.filter((resource) => resource.kind === "package");
	if (packages.length === 0) {
		return null;
	}
	return (
		<div className="resource-list">
			{packages.map((resource) => (
				<div className="resource-list-row" key={resource.id}>
					<span>
						<b>{resource.name}</b>
						<small>{resource.desired}</small>
					</span>
					<button
						type="button"
						disabled={isSaving}
						onClick={() => onEdit(resource)}
					>
						Edit {resource.name}
					</button>
					<button
						type="button"
						disabled={isSaving}
						onClick={() => onDelete(resource.id)}
					>
						Delete {resource.name}
					</button>
				</div>
			))}
		</div>
	);
}
