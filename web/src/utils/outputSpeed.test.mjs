import test from 'node:test';
import assert from 'node:assert/strict';
import { outputSpeed, outputSpeedDetail } from './outputSpeed.js';

test('the reported 4062.5 TPS log uses total time and non-reasoning output', () => {
  const row = { type: 2, completion_tokens: 65, request_time: 2879, metadata: { first_response: 2863, reasoning_tokens: 64 } };
  assert.equal(outputSpeed(row).tps.toFixed(2), '0.35');
  assert.equal(outputSpeed(row).measured, false);
  assert.match(outputSpeedDetail(row), /兼容估算/);
  row.metadata.upstream_response_ms = 2000;
  assert.equal(outputSpeed(row).tps, 0.5);
  row.metadata.first_response = 1;
  assert.equal(outputSpeed(row).tps, 0.5);
  assert.equal(outputSpeed(row).measured, true);
  row.metadata.upstream_response_ms = 0;
  assert.equal(outputSpeed(row), null);
});

test('no output, only reasoning and error records do not fabricate TPS', () => {
  assert.equal(outputSpeed(null), null);
  for (const row of [
    { type: 2, completion_tokens: 0, request_time: 1000 },
    { type: 2, completion_tokens: 65, request_time: 1000, metadata: { reasoning_tokens: 65 } },
    { type: 5, completion_tokens: 65, request_time: 1000 },
    { type: 2, completion_tokens: 65, request_time: 0 }
  ])
    assert.equal(outputSpeed(row), null);
});
