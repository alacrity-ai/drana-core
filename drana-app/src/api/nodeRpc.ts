import { NODE_RPC } from './config';
import { numericReviver } from './numericReviver';
import type { Account, NodeInfo, SubmitTxRequest, SubmitTxResponse, TxStatus, UnbondingResponse, Validator, TransactionResponse, MyStakesResponse, StakerInfo } from './types';

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${NODE_RPC}${path}`);
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    let msg = `HTTP ${res.status}`;
    try { msg = JSON.parse(text).error || msg; } catch {}
    throw new Error(msg);
  }
  const text = await res.text();
  return JSON.parse(text, numericReviver);
}

export const getNodeInfo = () => get<NodeInfo>('/v1/node/info');
export const getAccount = (addr: string) => get<Account>(`/v1/accounts/${addr}`);
export const getAccountByName = (name: string) => get<Account>(`/v1/accounts/name/${name}`);
export const getUnbonding = (addr: string) => get<UnbondingResponse>(`/v1/accounts/${addr}/unbonding`);
export const getValidators = () => get<Validator[]>('/v1/network/validators');

export const getTxStatus = (hash: string) => get<TxStatus>(`/v1/transactions/${hash}/status`);
export const getMempoolPending = (sender?: string) =>
  get<{ transactions: TransactionResponse[]; count: number }>(`/v1/mempool/pending${sender ? `?sender=${sender}` : ''}`);

export const getMyPostStakes = (addr: string) => get<MyStakesResponse>(`/v1/accounts/${addr}/post-stakes`);
export const getPostStakers = (postId: string) => get<{ stakers: StakerInfo[] }>(`/v1/posts/${postId}/stakers`);

export async function submitTransaction(req: SubmitTxRequest): Promise<SubmitTxResponse> {
  const res = await fetch(`${NODE_RPC}/v1/transactions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
  const text = await res.text();
  return JSON.parse(text, numericReviver);
}
