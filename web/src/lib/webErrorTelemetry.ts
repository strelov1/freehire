import { PUBLIC_OTEL_LOGS_ENDPOINT, PUBLIC_OTEL_ENVIRONMENT, PUBLIC_OTEL_SERVICE_VERSION } from '$app/env/public';
import { page } from '$app/state';
import { createErrorTelemetry } from './errorTelemetry.js';
import { markSigninIncident, takeSigninIncident } from './signinIncident.js';

export const webErrorTelemetry =
  typeof window === 'undefined'
    ? undefined
    : createErrorTelemetry({
        endpoint: PUBLIC_OTEL_LOGS_ENDPOINT,
        service: 'freehire-web',
        environment: PUBLIC_OTEL_ENVIRONMENT,
        version: PUBLIC_OTEL_SERVICE_VERSION,
        signinIncident: () => (page.data.user === null ? markSigninIncident() : undefined),
      });

export function reportSigninSession(): void {
  if (!webErrorTelemetry) return;
  const incidentID = takeSigninIncident();
  if (incidentID) webErrorTelemetry.reportSigninSession(incidentID);
}
