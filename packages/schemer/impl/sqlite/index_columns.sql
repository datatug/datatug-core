/*
-- noinspection SqlResolve
*/
SELECT
    cid,
    name
FROM PRAGMA_index_info(?)
ORDER BY seqno