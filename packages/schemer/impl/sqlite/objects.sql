-- noinspection SqlResolve
SELECT
    m.type,
    --m.name
    m.tbl_name,
    m.sql
FROM sqlite_master m
WHERE m.type in ('table', 'view')
ORDER BY m.name