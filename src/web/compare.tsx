import { memo, useDeferredValue, useMemo, useState } from "react";
import type {
	CompareMachine,
	CompareSection,
	CompareView,
	DesiredItem,
	DesiredManager,
} from "../shared/types";
import { AdoptButton, useManagedItems } from "./desired";
import {
	Empty,
	ErrorNotice,
	Loading,
	PageHeading,
	Status,
	useResource,
} from "./ui";

const SECTION_MANAGERS: Record<CompareSection["id"], DesiredManager | null> = {
	brewFormulae: "brew_formula",
	brewCasks: "brew_cask",
	pacman: "pacman",
	mise: "mise",
	files: null,
};

export function Compare() {
	const { data, error, loading, refresh } =
		useResource<CompareView>("/api/compare");
	const [differentOnly, setDifferentOnly] = useState(true);
	const { managed, refresh: refreshManaged } = useManagedItems();
	const [search, setSearch] = useState("");
	const query = useDeferredValue(search.trim().toLowerCase());
	return (
		<>
			<PageHeading
				eyebrow="CROSS-MACHINE VIEW"
				title="머신 비교"
				description="버전과 파일 해시를 나란히 확인합니다. 해당 항목을 수집하는 머신끼리만 비교합니다."
			>
				<button type="button" onClick={() => void refresh()}>
					새로고침
				</button>
			</PageHeading>
			<ErrorNotice message={error} retry={() => void refresh()} />
			<div className="compare-toolbar">
				<label className="checkbox-label">
					<input
						type="checkbox"
						checked={differentOnly}
						onChange={(event) => setDifferentOnly(event.target.checked)}
					/>
					다른 것만 보기
				</label>
				<label className="search-label">
					<span className="sr-only">비교 항목 검색</span>
					<input
						type="search"
						placeholder="패키지, 도구, 경로 검색…"
						value={search}
						onChange={(event) => setSearch(event.target.value)}
					/>
				</label>
				<div className="muted small compare-legend">
					<span className="difference-dot" />
					차이 있음 <span className="separator">·</span> — 미설치{" "}
					<span className="separator">·</span> 해당 없음 · 미수집
				</div>
			</div>
			{loading ? (
				<Loading />
			) : data?.machines.length ? (
				<div className="compare-sections">
					{data.sections.map((section) => (
						<Matrix
							key={section.id}
							section={section}
							machines={data.machines}
							differentOnly={differentOnly}
							query={query}
							managed={managed}
							onManagedChange={refreshManaged}
						/>
					))}
				</div>
			) : !error ? (
				<Empty title="비교할 머신이 없습니다.">
					<a href="#/" className="button primary">
						머신 추가하기
					</a>
				</Empty>
			) : null}
		</>
	);
}

const Matrix = memo(function Matrix({
	section,
	machines,
	differentOnly,
	query,
	managed,
	onManagedChange,
}: {
	section: CompareSection;
	machines: CompareMachine[];
	differentOnly: boolean;
	query: string;
	managed: Map<string, DesiredItem>;
	onManagedChange: () => void;
}) {
	const manager = SECTION_MANAGERS[section.id];
	const applicable = useMemo(
		() => new Set(section.machineIds),
		[section.machineIds],
	);
	const rows = useMemo(
		() =>
			section.rows.filter(
				(row) =>
					(!differentOnly || !row.uniform) &&
					(!query ||
						row.key.toLowerCase().includes(query) ||
						Object.values(row.cells).some((cell) =>
							cell?.toLowerCase().includes(query),
						)),
			),
		[section.rows, differentOnly, query],
	);
	const differences = section.rows.filter((row) => !row.uniform).length;
	return (
		<section
			className="panel compare-section"
			aria-labelledby={`section-${section.id}`}
		>
			<div className="section-heading">
				<div className="row-title">
					<h2 id={`section-${section.id}`}>{section.title}</h2>
					<span className="count-label mono">{rows.length}</span>
				</div>
				<span className="muted small">
					전체 {section.rows.length}개 · 차이 {differences}개 · 대상{" "}
					{applicable.size}대
				</span>
			</div>
			{rows.length ? (
				<section
					className="table-scroll matrix-scroll"
					aria-label={`${section.title} 비교 표 · 스크롤 가능`}
				>
					<table className="matrix">
						<thead>
							<tr>
								<th scope="col">{section.id === "files" ? "경로" : "항목"}</th>
								{machines.map((machine) => (
									<th
										key={machine.id}
										scope="col"
										className={
											applicable.has(machine.id) ? "" : "not-applicable"
										}
									>
										<a href={`#/machines/${machine.id}`}>{machine.name}</a>
										<div>
											<Status status={machine.status} />
										</div>
									</th>
								))}
							</tr>
						</thead>
						<tbody>
							{rows.map((row) => (
								<tr
									key={row.key}
									className={row.uniform ? "" : "different-row"}
								>
									<th scope="row" className="mono" title={row.key}>
										<div className="row-key-cell">
											<span>{row.key}</span>
											{manager && !row.dependencyOnly ? (
												<AdoptButton
													manager={manager}
													name={row.key}
													managed={managed}
													onChanged={onManagedChange}
												/>
											) : null}
										</div>
									</th>
									{machines.map((machine) => {
										const applies = applicable.has(machine.id);
										const value = row.cells[machine.id];
										return (
											<td
												key={machine.id}
												className={`mono ${applies ? (value == null ? "absent" : "") : "not-applicable"}`}
												title={
													applies
														? (value ?? "설치 또는 파일 없음")
														: "이 머신은 이 항목을 수집하지 않습니다."
												}
											>
												{applies ? (
													(value ?? "—")
												) : (
													<span className="small">해당 없음</span>
												)}
											</td>
										);
									})}
								</tr>
							))}
						</tbody>
					</table>
				</section>
			) : (
				<Empty
					title={
						query
							? "검색 조건에 맞는 항목이 없습니다."
							: differentOnly && section.rows.length
								? "대상 머신 사이에 차이가 없습니다."
								: "수집된 항목이 없습니다."
					}
				>
					{differentOnly && applicable.size === 1 ? (
						<p>
							수집 대상이 1대입니다. ‘다른 것만 보기’를 끄면 전체 목록을 볼 수
							있습니다.
						</p>
					) : null}
				</Empty>
			)}
		</section>
	);
});
