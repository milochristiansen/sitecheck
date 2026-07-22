function meta()
    return {
        name        = "Notification Demo",
        description = "Hits a random status code endpoint to demo ntfy notifications",
        notify      = {
            pass     = "siteCheckXYZZYtest",
            degraded = "siteCheckXYZZYtest",
            fail     = "siteCheckXYZZYtest",
        },
    }
end

function check()
    -- httpbin randomly picks one status from a comma-separated list,
    -- so each run may trigger a status transition and notify.
    local r = http_fetch("https://httpbin.org/status/200,404,503", {
        method = "GET",
        timeout = 10,
        follow_redirects = true,
    })

    if r.Error ~= nil then
        r.Pass = FAIL
        r.FailReason = r.Error
        return r
    end

    if r.StatusCode == 200 then
        r.Pass = PASS
        return r
    end

    if r.StatusCode == 404 then
        r.Pass = DEGRADED
        r.FailReason = "got 404"
        return r
    end

    -- 503 or any other non-200/404
    r.Pass = FAIL
    r.FailReason = "got " .. r.StatusCode
    return r
end
