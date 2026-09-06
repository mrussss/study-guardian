import type { ReactElement } from "react";
import { MOCK_SCENARIOS, type MockScenarioId } from "./supervisor";
import "./mock-toolbar.css";

export function MockScenarioToolbar({ scenario }: { scenario: MockScenarioId }): ReactElement {
  const changeScenario = (next: string): void => {
    const url = new URL(window.location.href);
    url.searchParams.set("mock", next);
    window.location.assign(url);
  };
  return <aside className="mock-scenario-toolbar" aria-label="浏览器 Mock 场景">
    <strong>Mock</strong>
    <select aria-label="Mock 场景" value={scenario} onChange={event => changeScenario(event.target.value)}>
      {MOCK_SCENARIOS.map(item => <option value={item.id} key={item.id}>{item.label}</option>)}
    </select>
  </aside>;
}
