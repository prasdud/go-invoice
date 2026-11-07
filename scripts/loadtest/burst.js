// Burst load: ramp to 5x the sustained target and back (500/min -> 2500/min).
//
// Run:
//   k6 run -e API_KEY=... scripts/loadtest/burst.js
//
// Pass criteria:
//   - no server errors (only 503 + Retry-After is acceptable during saturation)
//   - some invoices still succeed during the burst
//   - after the burst, the service recovers (verify /health and a manual POST)

import http from 'k6/http';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || 'changeme';

const created = new Counter('invoices_created');
const overloaded = new Counter('invoices_overloaded');
const serverErrors = new Counter('invoices_server_errors');

export const options = {
  scenarios: {
    burst: {
      executor: 'ramping-arrival-rate',
      startRate: 0,
      timeUnit: '1m',
      stages: [
        { target: 2500, duration: '30s' },
        { target: 2500, duration: '60s' },
        { target: 0, duration: '10s' },
      ],
      preAllocatedVUs: 100,
      maxVUs: 500,
    },
  },
  thresholds: {
    invoices_server_errors: ['count==0'],
    invoices_created: ['count>0'],
  },
};

const body = JSON.stringify({
  payload: {
    invoice_number: 'INV-BURST',
    items: [{ description: 'Burst item', quantity: 1, rate: 1.0 }],
  },
});

export default function () {
  const res = http.post(`${BASE_URL}/invoices`, body, {
    headers: { 'Content-Type': 'application/json', 'X-API-Key': API_KEY },
  });
  if (res.status === 200) {
    created.add(1);
  } else if (res.status === 503) {
    overloaded.add(1);
  } else {
    serverErrors.add(1);
  }
}
