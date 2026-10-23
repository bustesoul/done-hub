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

test('duration and first response share the request-send clock; legacy keeps its clock', async () => {
  const { responseDuration, firstResponse, responseTimeExplanation, outputSpeedExplanation } = await import('./outputSpeed.js');
  const row = { type: 2, is_stream: true, request_time: 8552, completion_tokens: 188,
    metadata: { first_response: 4837, upstream_response_ms: 8480.628079, response_metrics_version: 2, response_first_ms: 4820 } };
  assert.equal(responseDuration(row), 8480.628079);
  assert.equal(firstResponse(row), 4820);
  assert.equal(outputSpeed(row).tps.toFixed(2), '22.17');
  assert.match(outputSpeedDetail(row), /推理明细未提供/);
  row.is_stream = false;
  assert.equal(firstResponse(row), 4820);
  delete row.metadata.response_metrics_version;
  assert.equal(firstResponse(row), null);
  row.is_stream = true;
  assert.equal(firstResponse(row), 4837);
  assert.doesNotMatch(responseTimeExplanation + outputSpeedExplanation + outputSpeedDetail(row), /上游/);
});
