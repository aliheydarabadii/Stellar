import http from 'k6/http';
import { check, sleep } from 'k6';

const baseURL = __ENV.BASE_URL || 'http://localhost:8080';
const sleepSeconds = Number(__ENV.SLEEP_SECONDS || '0.2');

export const options = {
  scenarios: {
    http_health: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || '20'),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<250'],
  },
};

export default function () {
  const healthz = http.get(`${baseURL}/healthz`, {
    tags: { endpoint: 'healthz' },
  });
  check(healthz, {
    'healthz returns 200': (response) => response.status === 200,
  });

  const readyz = http.get(`${baseURL}/readyz`, {
    tags: { endpoint: 'readyz' },
  });
  check(readyz, {
    'readyz returns 200': (response) => response.status === 200,
  });

  sleep(sleepSeconds);
}
