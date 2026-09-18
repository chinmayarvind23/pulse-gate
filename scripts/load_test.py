import argparse, asyncio, os, time, uuid
from statistics import quantiles
import httpx
from common import encode_event, signed_headers

async def run(url: str, secret: str, rps: int, seconds: int) -> dict:
    """Generate a bounded open-loop load shape and retain latency samples for evidence.

    This is intentionally small and transparent. For final 10k+ RPS evidence, use k6 or
    vegeta from a separate host so the Python generator cannot become the bottleneck.
    """
    latencies, codes = [], {}
    sem = asyncio.Semaphore(max(256, rps // 2))
    async with httpx.AsyncClient(timeout=3.0, limits=httpx.Limits(max_connections=max(512, rps))) as client:
        async def one(i: int):
            async with sem:
                event={"event_id":f"load-{uuid.uuid4()}","amount_cents":1000+(i%100000),"currency":"USD","merchant_category":5812,"country":"US","card_present":bool(i%2),"hour_utc":i%24,"velocity_5m":i%8,"velocity_1h":i%20,"prior_declines":i%3}
                body=encode_event(event); start=time.perf_counter()
                try: resp=await client.post(url, content=body, headers=signed_headers(secret, body)); code=resp.status_code
                except Exception: code=0
                latencies.append((time.perf_counter()-start)*1000); codes[code]=codes.get(code,0)+1
        started=time.perf_counter(); tasks=[]
        for tick in range(seconds*10):
            batch=max(1,rps//10); tasks.extend(asyncio.create_task(one(tick*batch+j)) for j in range(batch)); await asyncio.sleep(0.1)
        await asyncio.gather(*tasks); elapsed=time.perf_counter()-started
    ordered=sorted(latencies)
    def pct(p): return ordered[min(len(ordered)-1, int(len(ordered)*p))] if ordered else 0
    return {"requests":len(ordered),"elapsed_s":elapsed,"observed_rps":len(ordered)/elapsed if elapsed else 0,"p50_ms":pct(.50),"p95_ms":pct(.95),"p99_ms":pct(.99),"codes":codes}

def main():
    p=argparse.ArgumentParser(); p.add_argument('--url',default='http://localhost:8080/v1/events'); p.add_argument('--rps',type=int,default=1000); p.add_argument('--seconds',type=int,default=10); a=p.parse_args()
    print(asyncio.run(run(a.url, os.getenv('PULSEGATE_HMAC_SECRET','dev-secret-change-me'), a.rps, a.seconds)))
if __name__=='__main__': main()
