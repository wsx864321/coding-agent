#!/usr/bin/env python3
"""
MySQL General Log Analysis Script
"""
import re
import sys
from collections import Counter
from datetime import datetime

LOG_PATH = r"D:\chromdownload\mysql_general.log"

SQL_KW = frozenset({
    'select', 'from', 'where', 'and', 'or', 'set', 'values', 'into', 'limit',
    'order', 'group', 'by', 'in', 'not', 'null', 'is', 'like', 'as', 'on',
    'inner', 'left', 'right', 'join', 'asc', 'desc', 'insert', 'update',
    'delete', 'replace', 'between', 'exists', 'having', 'distinct', 'union',
    'all', 'count', 'sum', 'avg', 'max', 'min', 'primary', 'key', 'create',
    'alter', 'drop', 'index', 'unique', 'default', 'int', 'varchar', 'text',
    'bigint', 'enum', 'datetime', 'timestamp', 'tinyint', 'smallint',
    'mediumint', 'float', 'double', 'decimal', 'char', 'blob', 'json',
    'use', 'show', 'explain', 'call', 'begin', 'commit', 'rollback',
    'truncate', 'grant', 'revoke', 'lock', 'unlock', 'tables',
    'description', 'describe', 'partition', 'handler', 'load',
    'cascade', 'references', 'foreign', 'constraint', 'check',
    'if', 'else', 'then', 'end', 'case', 'when', 'using',
    'do', 'while', 'repeat', 'loop', 'leave', 'iterate',
    'declare', 'cursor', 'fetch', 'open', 'close',
    'schema', 'database', 'databases', 'table', 'column', 'columns',
    'view', 'function', 'procedure', 'trigger', 'event',
    'engine', 'charset', 'collation', 'status', 'variables',
    'processlist', 'profiling', 'warnings', 'errors',
    'global', 'session', 'local', 'storage', 'memory',
    'year', 'month', 'day', 'hour', 'minute', 'second',
    'current_timestamp', 'now', 'utc_timestamp', 'utc_date',
    'name', 'id', 'type', 'extra', 'size', 'version', 'ctime', 'mtime',
    'date', 'status', 'source', 'file_type', 'file_num', 'file_size',
    'ext', 'drive_id', 'company_id', 'creator_id', 'modifier_id',
    'parent_id', 'allotee_id',
})

def extract_tables(sql):
    tables = []
    all_quoted = re.findall(r'`(\w+?)`\.`(\w+?)`', sql)
    for db, tbl in all_quoted:
        if db.lower() not in SQL_KW and tbl.lower() not in SQL_KW:
            tables.append(f"{db}.{tbl}")

    single = re.findall(r'(?<![.`])`(\w+?)`(?!\.)', sql)
    for tbl in single:
        if tbl.lower() not in SQL_KW:
            tables.append(tbl)
    return list(set(tables)) if tables else []


def parse_log(filepath):
    records = []
    with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            if line.startswith(('/usr/', 'Tcp port', 'Time ')):
                continue

            parts = line.split('\t')
            if len(parts) < 2:
                continue

            ts_str = parts[0].strip()
            try:
                ts = datetime.fromisoformat(ts_str.rstrip('Z'))
            except:
                continue

            # parts[1] = "connId Command"
            conn_cmd = parts[1].strip()
            sql = parts[2].strip() if len(parts) > 2 else ''

            # Split "430072 Execute" into conn_id and cmd
            cc_parts = conn_cmd.split(' ', 1)
            if len(cc_parts) == 2:
                conn_id, cmd = cc_parts
            else:
                conn_id = cc_parts[0]
                cmd = 'Unknown'

            db_tables = extract_tables(sql) if sql else []
            records.append({
                'ts': ts, 'conn': conn_id, 'cmd': cmd,
                'sql': sql, 'tables': db_tables
            })
    return records


def main():
    sys.stdout.reconfigure(encoding='utf-8')

    print("=" * 75)
    print("  MySQL General Log Analysis Report")
    print("=" * 75)
    print(f"  File: {LOG_PATH}")

    records = parse_log(LOG_PATH)
    total = len(records)
    if total == 0:
        print("  No valid records found.")
        return

    start_ts = records[0]['ts']
    end_ts = records[-1]['ts']
    duration = (end_ts - start_ts).total_seconds()

    print(f"\n  [Basic Info]")
    print(f"    Total Records : {total:>10,}")
    print(f"    Time Range    : {start_ts} ~ {end_ts}")
    print(f"    Duration      : {duration:.1f}s ({duration/60:.1f}min)")
    print(f"    Average QPS   : {total/duration:>10.1f}")

    # ---- 1. Command Type ----
    print(f"\n{'='*75}")
    print(f"  [1] Command Type Breakdown")
    print(f"{'='*75}")
    cmd_counter = Counter(r['cmd'] for r in records)
    print(f"  {'Command':<20} {'Count':>10} {'Pct':>8} {'QPS':>8}")
    print(f"  {'-'*20} {'-'*10} {'-'*8} {'-'*8}")
    for cmd, cnt in cmd_counter.most_common():
        pct = cnt / total * 100
        qps = cnt / duration if duration > 0 else 0
        print(f"  {cmd:<20} {cnt:>10,} {pct:>7.1f}% {qps:>7.1f}")

    # ---- 2. Tables ----
    exec_records = [r for r in records if r['cmd'] in ('Execute', 'Query')]
    print(f"\n  [2] Table Hotspots (Execute/Query, {len(exec_records):,} operations)")
    table_counter = Counter()
    for r in exec_records:
        for t in r['tables']:
            table_counter[t] += 1
    no_table_count = sum(1 for r in exec_records if not r['tables'])

    if table_counter:
        print(f"  {'DB.Table':<55} {'Refs':>8} {'Pct':>7}")
        print(f"  {'-'*55} {'-'*8} {'-'*7}")
        for tbl, cnt in table_counter.most_common(30):
            pct = cnt / len(exec_records) * 100
            print(f"  {tbl[:55]:<55} {cnt:>8,} {pct:>6.1f}%")
        if no_table_count > 0:
            print(f"  {'(others / no table extracted)':<55} {no_table_count:>8,} {no_table_count/len(exec_records)*100:>6.1f}%")

    # ---- 3. SQL Patterns ----
    print(f"\n  [3] Top 30 SQL Patterns (normalized)")
    sql_counter = Counter()
    for r in exec_records:
        s = r['sql']
        s = re.sub(r"'[^']*'", '?', s)
        s = re.sub(r'\b\d+\b', '?', s)
        s = re.sub(r'\s+', ' ', s).strip()
        if len(s) > 140:
            s = s[:137] + '...'
        sql_counter[s] += 1

    print(f"  {'Count':>8}  {'SQL Pattern'}")
    print(f"  {'-'*8}  {'-'*60}")
    for sql, cnt in sql_counter.most_common(30):
        print(f"  {cnt:>8,}  {sql}")

    # ---- 4. Per-Minute QPS ----
    print(f"\n  [4] Per-Minute QPS")
    min_counter = Counter()
    for r in exec_records:
        min_counter[r['ts'].strftime('%H:%M')] += 1
    print(f"  {'Minute':<8} {'Req':>8} {'RPS':>8}")
    print(f"  {'-'*8} {'-'*8} {'-'*8}")
    for m, cnt in sorted(min_counter.items())[:30]:
        print(f"  {m:<8} {cnt:>8,} {cnt/60:>7.1f}")

    # ---- 5. Per-Second QPS ----
    print(f"\n  [5] Per-Second QPS Distribution")
    sec_counter = Counter()
    for r in exec_records:
        sec_counter[r['ts'].strftime('%H:%M:%S')] += 1
    if sec_counter:
        vals = sorted(sec_counter.values())
        n = len(vals)
        print(f"     Peak QPS : {max(vals)}")
        print(f"     Mean QPS : {sum(vals)/n:.1f}")
        print(f"     Min  QPS : {min(vals)}")
        print(f"     P50  QPS : {vals[n//2]}")
        print(f"     P95  QPS : {vals[int(n*0.95)]}")
        print(f"     P99  QPS : {vals[int(n*0.99)]}")
        print(f"\n     Top 10 Peak Seconds:")
        for ts_sec, cnt in sec_counter.most_common(10):
            print(f"       {ts_sec}  |  {cnt} req/s")

    # ---- 6. Connection IDs ----
    print(f"\n  [6] Top 15 Connection IDs")
    conn_counter = Counter(r['conn'] for r in records)
    print(f"  {'ConnID':>8} {'Total':>10} {'Execute':>9} {'Query':>8} {'Prepare':>9} {'Close':>8} {'Other':>8}")
    print(f"  {'-'*8} {'-'*10} {'-'*9} {'-'*8} {'-'*9} {'-'*8} {'-'*8}")
    for cid, cnt in conn_counter.most_common(15):
        cc = Counter(r['cmd'] for r in records if r['conn'] == cid)
        print(f"  {cid:>8} {cnt:>10,} "
              f"{cc.get('Execute',0):>9} "
              f"{cc.get('Query',0):>8} "
              f"{cc.get('Prepare',0):>9} "
              f"{cc.get('Close stmt',0):>8} "
              f"{sum(v for k,v in cc.items() if k not in ('Execute','Query','Prepare','Close stmt')):>8}")

    print(f"\n     Total unique connections: {len(conn_counter)}")

    # ---- 7. Database perspective ----
    print(f"\n  [7] Database / Table Group Summary")
    db_counter = Counter()
    for r in exec_records:
        for t in r['tables']:
            if '.' in t:
                db = t.split('.')[0]
                db_counter[db] += 1

    # Group by table prefix (e.g. tb_file_*)
    prefix_counter = Counter()
    for r in exec_records:
        for t in r['tables']:
            if not t.startswith('<'):
                m = re.match(r'^(tb_\w+?)(?:_\d+)?$', t)
                if m:
                    prefix_counter[f"{m.group(1)}_*"] += 1
                elif '.' in t:
                    prefix_counter[t] += 1
                else:
                    prefix_counter[t] += 1

    if prefix_counter:
        print(f"\n     Table Groups (with shard suffix merged):")
        print(f"     {'Table':<55} {'Hits':>8}")
        print(f"     {'-'*55} {'-'*8}")
        for tbl, cnt in prefix_counter.most_common(20):
            print(f"     {tbl[:55]:<55} {cnt:>8,}")

    print(f"\n{'='*75}")
    print(f"  Analysis Complete")
    print(f"{'='*75}")


if __name__ == '__main__':
    main()
