import { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import { Compare } from "./compare";
import { Detail } from "./detail";
import { Machines } from "./machines";
import { Settings } from "./settings";
import { Empty, useResource } from "./ui";
import "./styles.css";

function App() {
	const [route, setRoute] = useState(window.location.hash.slice(1) || "/");
	const { data: identity } = useResource<{ login: string }>("/api/me");
	useEffect(() => {
		const change = () => {
			setRoute(window.location.hash.slice(1) || "/");
			window.scrollTo(0, 0);
		};
		window.addEventListener("hashchange", change);
		return () => window.removeEventListener("hashchange", change);
	}, []);
	const detailMatch = /^\/machines\/(\d+)$/.exec(route);
	const page =
		route === "/compare"
			? "compare"
			: route === "/settings"
				? "settings"
				: "machines";
	useEffect(() => {
		document.title = `${page === "compare" ? "머신 비교" : page === "settings" ? "허브 설정" : "내 머신"} · 늘`;
	}, [page]);
	return (
		<>
			<button
				type="button"
				className="skip-link"
				onClick={() => document.getElementById("main")?.focus()}
			>
				본문으로 이동
			</button>
			<header className="app-header">
				<div className="header-inner">
					<a className="brand" href="#/" aria-label="늘 · 머신 목록">
						<span className="brand-mark">늘</span>
						<span className="brand-description">개발 머신 허브</span>
					</a>
					<nav aria-label="주 메뉴">
						<a
							href="#/"
							className={page === "machines" ? "active" : ""}
							aria-current={page === "machines" ? "page" : undefined}
						>
							머신
						</a>
						<a
							href="#/compare"
							className={page === "compare" ? "active" : ""}
							aria-current={page === "compare" ? "page" : undefined}
						>
							비교
						</a>
						<a
							href="#/settings"
							className={page === "settings" ? "active" : ""}
							aria-current={page === "settings" ? "page" : undefined}
						>
							설정
						</a>
					</nav>
					<span className="hub-label mono" title={identity?.login}>
						<span className="status-dot" />
						{identity?.login === "local" || !identity
							? "LOCAL HUB"
							: identity.login}
					</span>
				</div>
			</header>
			<main id="main" tabIndex={-1} className="app-main">
				{detailMatch ? (
					<Detail key={detailMatch[1]} id={Number(detailMatch[1])} />
				) : route === "/" ? (
					<Machines />
				) : route === "/compare" ? (
					<Compare />
				) : route === "/settings" ? (
					<Settings />
				) : (
					<Empty title="페이지를 찾을 수 없습니다.">
						<a href="#/">머신 목록으로 돌아가기</a>
					</Empty>
				)}
			</main>
			<footer className="app-footer">
				<span>
					늘 <span className="separator">/</span> 개인 개발 환경 관측소
				</span>
				<span className="mono">READ-ONLY INVENTORY</span>
			</footer>
		</>
	);
}

const root = document.getElementById("root");
if (!root) throw new Error("Missing #root element");
createRoot(root).render(<App />);
