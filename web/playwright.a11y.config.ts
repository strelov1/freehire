import { defineConfig, devices } from '@playwright/test';
import { MOCK_API_PORT } from './e2e/support/mock-api.ts';

// The accessibility check, the one e2e spec that needs no running stack: it starts the
// production build (`pnpm build` first) against a stand-in for the API, so it is the same
// command on a laptop and in CI. The rest of e2e/ talks to a real stack and is not run here.
const APP_PORT = Number(process.env.A11Y_APP_PORT ?? 8090);
const MOCK_API_URL = `http://127.0.0.1:${MOCK_API_PORT}`;

export default defineConfig({
	testDir: 'e2e',
	testMatch: 'a11y.spec.ts',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	reporter: 'list',
	use: {
		baseURL: `http://127.0.0.1:${APP_PORT}`,
		trace: 'on-first-retry'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: [
		{
			command: 'node e2e/support/mock-api.ts',
			url: `${MOCK_API_URL}/api/v1/stats/catalog`,
			env: { MOCK_API_PORT: String(MOCK_API_PORT) }
		},
		{
			command: 'node build/index.js',
			url: `http://127.0.0.1:${APP_PORT}/signin`,
			env: {
				PORT: String(APP_PORT),
				HOST: '127.0.0.1',
				API_INTERNAL_URL: MOCK_API_URL
			}
		}
	]
});
