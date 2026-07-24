function meta()
    return {
        skip        = true,
        name        = "HTTP JSON Body",
        description = "Parses a JSON endpoint body, extracts fields, and validates structure",
    }
end

function check()
    local r = http_fetch("https://httpbin.org/json", {
        method = "GET",
        timeout = 10,
        follow_redirects = true,
    })

    if r.Error ~= nil then
        r.FailReason = r.Error
        return r
    end

    if r.StatusCode ~= 200 then
        r.Pass = DEGRADED
        r.FailReason = "unexpected status " .. r.StatusCode
        return r
    end

    local data = json.parse(r.Body)
    if data == nil then
        r.Pass = DEGRADED
        r.FailReason = "failed to parse JSON body"
        return r
    end

    local slideshow = data.slideshow
    if slideshow == nil then
        r.Pass = DEGRADED
        r.FailReason = "missing slideshow in JSON"
        return r
    end

    r.Pass = PASS
    return r
end
