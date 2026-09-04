export const CONTROL_CENTER_ROUTE_EVENT = "studyguardian://control-center-route";

export const CONTROL_CENTER_ROUTES = [
  "overview",
  "missions",
  "achievements",
  "rewards",
  "review",
  "history",
  "settings",
  "system",
] as const;

export type ControlCenterRoute = typeof CONTROL_CENTER_ROUTES[number];

export function isControlCenterRoute(value: unknown): value is ControlCenterRoute {
  return typeof value === "string" && (CONTROL_CENTER_ROUTES as readonly string[]).includes(value);
}
