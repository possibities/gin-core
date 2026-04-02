DELETE FROM casbin_rule
WHERE ptype = 'p'
  AND v0 = 'admin'
  AND v1 = '/api/v1/admin/*'
  AND v2 = 'GET';
