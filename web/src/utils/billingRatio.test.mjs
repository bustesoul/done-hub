import test from 'node:test';
import assert from 'node:assert/strict';
import { billingRatio } from './billingRatio.js';

test('new bills use frozen channel rate including service tier; historical bills keep group rate', () => {
  assert.equal(billingRatio({ billing_version: 2, channel_ratio: .5, group_ratio: 9, service_tier_ratio: 1.5, effective_ratio: .75 }), .75);
  assert.equal(billingRatio({ group_ratio: .5, service_tier_ratio: 1.5 }), .75);
  assert.equal(billingRatio({ group_ratio: 0 }), 0);
  assert.equal(billingRatio(), 1);
});
