#include <gtest/gtest.h>

#include "nats/Frame.hpp"

using namespace wp::nats;

TEST(Parser, InfoFrame) {
    Parser p;
    p.feed("INFO {\"server_id\":\"x\"}\r\n");
    auto fr = p.next();
    ASSERT_TRUE(fr.has_value());
    EXPECT_EQ(fr->kind, FrameKind::Info);
    EXPECT_EQ(fr->payload, "{\"server_id\":\"x\"}");
}

TEST(Parser, PingPong) {
    Parser p;
    p.feed("PING\r\nPONG\r\n");
    auto a = p.next();
    auto b = p.next();
    ASSERT_TRUE(a.has_value());
    ASSERT_TRUE(b.has_value());
    EXPECT_EQ(a->kind, FrameKind::Ping);
    EXPECT_EQ(b->kind, FrameKind::Pong);
}

TEST(Parser, MsgFrame_FourParts_NoReply) {
    Parser p;
    p.feed("MSG hello 1 2\r\nhi\r\n");
    auto fr = p.next();
    ASSERT_TRUE(fr.has_value());
    EXPECT_EQ(fr->kind, FrameKind::Msg);
    EXPECT_EQ(fr->subject, "hello");
    EXPECT_EQ(fr->sid, "1");
    EXPECT_EQ(fr->reply, "");
    EXPECT_EQ(fr->payload, "hi");
}

TEST(Parser, MsgFrame_FiveParts_WithReply) {
    Parser p;
    p.feed("MSG hello 1 _INBOX.abc 2\r\nhi\r\n");
    auto fr = p.next();
    ASSERT_TRUE(fr.has_value());
    EXPECT_EQ(fr->kind, FrameKind::Msg);
    EXPECT_EQ(fr->subject, "hello");
    EXPECT_EQ(fr->sid, "1");
    EXPECT_EQ(fr->reply, "_INBOX.abc");
    EXPECT_EQ(fr->payload, "hi");
}

TEST(Parser, PartialThenComplete) {
    Parser p;
    p.feed("MSG x ");
    EXPECT_FALSE(p.next().has_value());
    p.feed("1 2\r\nhi\r\n");
    auto fr = p.next();
    ASSERT_TRUE(fr.has_value());
    EXPECT_EQ(fr->payload, "hi");
}

TEST(Parser, OkErr) {
    Parser p;
    p.feed("+OK\r\n-ERR 'bad subj'\r\n");
    auto a = p.next();
    auto b = p.next();
    ASSERT_TRUE(a.has_value());
    ASSERT_TRUE(b.has_value());
    EXPECT_EQ(a->kind, FrameKind::Ok);
    EXPECT_EQ(b->kind, FrameKind::Err);
    EXPECT_EQ(b->payload, "'bad subj'");
}
