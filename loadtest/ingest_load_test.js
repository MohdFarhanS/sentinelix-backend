import http from 'k6/http';
import { check } from 'k6';
import { Rate } from 'k6/metrics';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080/api/v1';
const THROUGHPUT_KEY_COUNT = 100;

const rateLimited = new Rate('rate_limited_events');
const unexpectedErrors = new Rate('unexpected_errors');

export const options = {
  scenarios: {
    throughput: {
      executor: 'constant-arrival-rate',
      rate: 100,
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 50,
      maxVUs: 150,
      exec: 'throughputTest',
      startTime: '0s',
    },
    rate_limit_overload: {
      executor: 'constant-arrival-rate',
      rate: 10,
      timeUnit: '1s',
      duration: '20s',
      preAllocatedVUs: 5,
      maxVUs: 20,
      exec: 'overloadTest',
      startTime: '35s',
    },
  },
  thresholds: {
    'http_req_duration{scenario:throughput}': ['p(95) < 300'],
    'http_req_failed{scenario:throughput}': ['rate < 0.01'],
    'unexpected_errors{scenario:rate_limit_overload}': ['rate == 0'],
    'rate_limited_events{scenario:rate_limit_overload}': ['rate > 0.3'],
  },
};

function buildEventPayload(i) {
  return JSON.stringify({
    level: 'error',
    message: `LoadTestError: simulated failure #${i}`,
    stack_trace: `at loadtest.js:${i}\n  at k6/execution`,
    context: { user_id: `loadtest-${i}`, route: '/loadtest' },
  });
}

export function setup() {
  const jar = http.cookieJar();
  const email = `k6-loadtest-${Date.now()}@sentinelix.com`;
  const password = 'Secret123!';

  const registerRes = http.post(
    `${BASE_URL}/auth/register`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } }
  );
  if (registerRes.status !== 201) {
    throw new Error(`Setup gagal: register status ${registerRes.status}, body: ${registerRes.body}`);
  }

  const loginRes = http.post(
    `${BASE_URL}/auth/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' }, jar }
  );
  if (loginRes.status !== 200) {
    throw new Error(`Setup gagal: login status ${loginRes.status}, body: ${loginRes.body}`);
  }

  // projectIds dikumpulin SEKALIAN dengan apiKeys — dipakai teardown()
  // buat DELETE satu-satu (DELETE /projects/:id butuh id, bukan api_key).
  const apiKeys = [];
  const projectIds = [];
  for (let i = 0; i < THROUGHPUT_KEY_COUNT; i++) {
    const res = http.post(
      `${BASE_URL}/projects`,
      JSON.stringify({ name: `k6-loadtest-project-${i}` }),
      { headers: { 'Content-Type': 'application/json' }, jar }
    );
    if (res.status !== 201) {
      throw new Error(`Setup gagal bikin project #${i}: status ${res.status}, body: ${res.body}`);
    }
    apiKeys.push(res.json('api_key'));
    projectIds.push(res.json('id'));
  }

  const overloadProjectRes = http.post(
    `${BASE_URL}/projects`,
    JSON.stringify({ name: 'k6-loadtest-project-overload' }),
    { headers: { 'Content-Type': 'application/json' }, jar }
  );
  if (overloadProjectRes.status !== 201) {
    throw new Error(`Setup gagal bikin project overload: status ${overloadProjectRes.status}`);
  }
  projectIds.push(overloadProjectRes.json('id'));

  console.log(`Setup selesai: ${apiKeys.length} API key buat throughput test, 1 API key terpisah buat overload test.`);

  // email & password ikut di-return — teardown() jalan di context TERPISAH
  // dari setup() (bukan VU yang sama), cookie jar dari sini TIDAK otomatis
  // kebawa ke teardown(). Login ulang di teardown() pakai kredensial ini,
  // bukan asumsi cookie jar-nya masih hidup.
  return { apiKeys, overloadKey: overloadProjectRes.json('api_key'), projectIds, email, password };
}

export function throughputTest(data) {
  const iter = exec.scenario.iterationInTest;
  const apiKey = data.apiKeys[iter % data.apiKeys.length];

  const res = http.post(`${BASE_URL}/ingest/event`, buildEventPayload(iter), {
    headers: {
      'Content-Type': 'application/json',
      'X-SentinelIX-Key': apiKey,
    },
  });

  check(res, {
    'throughput: status 202 (accepted)': (r) => r.status === 202,
  });
}

export function overloadTest(data) {
  const iter = exec.scenario.iterationInTest;

  const res = http.post(`${BASE_URL}/ingest/event`, buildEventPayload(iter), {
    headers: {
      'Content-Type': 'application/json',
      'X-SentinelIX-Key': data.overloadKey,
    },
  });

  rateLimited.add(res.status === 429);
  unexpectedErrors.add(res.status !== 202 && res.status !== 429);

  check(res, {
    'overload: status HANYA 202 atau 429 (tidak crash)': (r) => r.status === 202 || r.status === 429,
  });
}

// teardown() jalan SEKALI di akhir, setelah SEMUA scenario selesai — tidak
// dihitung sebagai bagian dari load yang diukur, sama seperti setup().
//
// KETERBATASAN JUJUR: tidak ada endpoint DELETE buat user di
// 04-API-DESIGN.md (cuma DELETE /projects/:id) — jadi ini cuma
// membersihkan 101 project (cascade ke alert_rules/monitors/issues/events
// per project, sesuai ON DELETE CASCADE di 03-DATABASE-DESIGN.md). Baris
// user itu sendiri TETAP TERSISA di tabel users, di luar kemampuan
// dibersihkan lewat API yang ada sekarang.
export function teardown(data) {
  const jar = http.cookieJar();
  const loginRes = http.post(
    `${BASE_URL}/auth/login`,
    JSON.stringify({ email: data.email, password: data.password }),
    { headers: { 'Content-Type': 'application/json' }, jar }
  );
  if (loginRes.status !== 200) {
    console.error(`Teardown gagal login ulang (status ${loginRes.status}) — project k6 TIDAK dibersihkan, hapus manual lewat SQL (lihat catatan di bawah).`);
    return;
  }

  let deleted = 0;
  let failed = 0;
  for (const projectId of data.projectIds) {
    const res = http.del(`${BASE_URL}/projects/${projectId}`, null, { jar });
    if (res.status === 204) {
      deleted++;
    } else {
      failed++;
      console.error(`Gagal hapus project ${projectId}: status ${res.status}`);
    }
  }

  console.log(`Teardown selesai: ${deleted} project terhapus, ${failed} gagal.`);
  console.log(`Catatan: baris user (${data.email}) TETAP ADA di DB — tidak ada endpoint DELETE user. Bersihkan manual kalau perlu:`);
  console.log(`  DELETE FROM users WHERE email LIKE 'k6-loadtest-%';`);
}