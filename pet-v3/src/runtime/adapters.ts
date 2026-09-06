import {
  NativeSupervisorControlAdapter, NativeSupervisorDashboardAdapter, NativeSystemIntegrationAdapter,
  type SupervisorControlAdapter, type SupervisorDashboardAdapter, type SystemIntegrationAdapter,
} from "../transport/supervisor";
import { MockSupervisorRuntime, parseMockScenario, type MockScenarioId } from "../mock/supervisor";

export const isTauriRuntime = typeof window !== "undefined"
  && Object.prototype.hasOwnProperty.call(window, "__TAURI_INTERNALS__");
const mockScenario: MockScenarioId | undefined = isTauriRuntime || typeof window === "undefined" || !import.meta.env.DEV
  ? undefined : parseMockScenario(window.location.search);
const mockRuntime = mockScenario ? new MockSupervisorRuntime(mockScenario) : undefined;

export function getSupervisorDashboardAdapter(): SupervisorDashboardAdapter {
  return mockRuntime ?? new NativeSupervisorDashboardAdapter();
}
export function getSupervisorControlAdapter(): SupervisorControlAdapter {
  return mockRuntime ?? new NativeSupervisorControlAdapter();
}
export function getSystemIntegrationAdapter(): SystemIntegrationAdapter {
  return mockRuntime ?? new NativeSystemIntegrationAdapter();
}
export function getMockScenario(): MockScenarioId | undefined { return mockScenario; }
