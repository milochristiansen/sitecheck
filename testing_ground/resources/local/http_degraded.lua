function meta()
    return {
        name        = "HTTP Degraded",
        description = "Checks the slow endpoint and marks DEGRADED if response time is high",
    }
end

function check()
    local r = http_fetch("http://127.0.0.1:19976/slow", {
        method = "GET",
        timeout = 10,
    })

    if r.Error ~= nil and r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.StatusCode ~= 200 then
        r.Pass = FAIL
        r.FailReason = "expected 200, got " .. r.StatusCode
        return r
    end

    -- The /slow endpoint has a 2-second delay by default.
    -- Use DEGRADED if it took longer than 1 second.
    if r.ResponseTimeMS > 1000 then
        r.Pass = DEGRADED
        r.FailReason = "response time " .. string.format("%.0f", r.ResponseTimeMS) .. "ms exceeds 1000ms threshold"
        return r
    end

    r.Pass = PASS
    return r
end
