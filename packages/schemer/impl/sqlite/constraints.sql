/*
-- noinspection SqlResolve
*/
select
    --[id],
    --[seq],
    [table],
    [from],
    [to],
    [on_update],
    [on_delete],
    [match]
from PRAGMA_foreign_key_list(?)
