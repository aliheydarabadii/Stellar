import http from 'k6/http';
import { check } from 'k6';

const influxURL = __ENV.INFLUX_URL || 'http://localhost:8086';
const influxOrg = __ENV.INFLUX_ORG || 'local';
const influxBucket = __ENV.INFLUX_BUCKET || 'telemetry';
const influxToken = __ENV.INFLUX_TOKEN || 'dev-token';
const assetID = __ENV.ASSET_ID || '871689260010377213';
const setpoint = Number(__ENV.SETPOINT || '100');
const activePower = Number(__ENV.ACTIVE_POWER || '55');

const writeURL =
  `${influxURL}/api/v2/write` +
  `?org=${encodeURIComponent(influxOrg)}` +
  `&bucket=${encodeURIComponent(influxBucket)}` +
  `&precision=ns`;

export const options = {
  scenarios: {
    influx_write: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || '20'),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

export default function () {
  const timestamp = `${Date.now()}000000`;
  const body =
    `asset_measurements,asset_id=${assetID}` +
    ` active_power=${activePower},setpoint=${setpoint} ${timestamp}`;

  const response = http.post(writeURL, body, {
    headers: {
      Authorization: `Token ${influxToken}`,
      'Content-Type': 'text/plain; charset=utf-8',
      Accept: 'application/json',
    },
    tags: {
      target: 'influxdb',
    },
  });

  check(response, {
    'influx write returns 204': (res) => res.status === 204,
  });
}
