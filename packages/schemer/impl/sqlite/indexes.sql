-- noinspection SqlResolve
SELECT
    -- seq,
    name,
    [unique],
    origin,
    partial
FROM PRAGMA_index_list('Album') ORDER BY seq;
