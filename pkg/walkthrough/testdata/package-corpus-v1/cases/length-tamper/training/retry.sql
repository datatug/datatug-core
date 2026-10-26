select status, count(*) from retries where attempts >= @attempts group by status;
